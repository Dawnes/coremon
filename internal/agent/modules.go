package agent

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"time"

	"coremon/internal/proto"
	"coremon/internal/sign"
)

var moduleNameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

type moduleManager struct {
	dir   string
	locks sync.Map // module name -> *sync.Mutex
}

func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func (m *moduleManager) binPath(name string) string {
	return filepath.Join(m.dir, name, exeName(name))
}

func (m *moduleManager) lock(name string) *sync.Mutex {
	l, _ := m.locks.LoadOrStore(name, &sync.Mutex{})
	return l.(*sync.Mutex)
}

// list returns installed modules with their manifests.
func (m *moduleManager) list() []proto.ModuleInfo {
	entries, _ := os.ReadDir(m.dir)
	out := []proto.ModuleInfo{}
	for _, e := range entries {
		if !e.IsDir() || !moduleNameRe.MatchString(e.Name()) {
			continue
		}
		info := proto.ModuleInfo{Name: e.Name()}
		var man proto.Manifest
		if err := loadJSON(filepath.Join(m.dir, e.Name(), "manifest.json"), &man); err != nil {
			info.Error = "manifest unreadable"
		} else {
			info.Version = man.Version
			info.Manifest = &man
		}
		out = append(out, info)
	}
	return out
}

func readManifest(ctx context.Context, bin string) (*proto.Manifest, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "manifest").Output()
	if err != nil {
		return nil, fmt.Errorf("module manifest: %w", err)
	}
	var man proto.Manifest
	if err := json.Unmarshal(out, &man); err != nil {
		return nil, fmt.Errorf("module manifest: %w", err)
	}
	return &man, nil
}

// fetchVerified downloads an artifact and verifies hash and signature.
func fetchVerified(ctx context.Context, cl *client, pub string, a *proto.Artifact, dst string) error {
	if a == nil {
		return errors.New("missing artifact")
	}
	key, err := sign.ParsePublic(pub)
	if err != nil {
		return err
	}
	if a.OS != runtime.GOOS || a.Arch != runtime.GOARCH {
		return fmt.Errorf("artifact is for %s/%s, this system is %s/%s", a.OS, a.Arch, runtime.GOOS, runtime.GOARCH)
	}
	// The signature is checked first against the advertised hash, then the
	// downloaded bytes are checked against that hash.
	if !sign.Verify(key, sign.Message(a.Kind, a.Name, a.Version, a.OS, a.Arch, a.SHA256), a.Signature) {
		return errors.New("signature verification FAILED - refusing artifact")
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
	if err != nil {
		return err
	}
	h := sha256.New()
	err = cl.download(ctx, a.URL, io.MultiWriter(f, h), 256<<20)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(dst)
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != a.SHA256 {
		os.Remove(dst)
		return fmt.Errorf("sha256 mismatch: got %s", got)
	}
	return nil
}

func (m *moduleManager) install(ctx context.Context, cl *client, pub string, a *proto.Artifact, logf func(string, ...any)) error {
	if a == nil || !moduleNameRe.MatchString(a.Name) || a.Kind != "module" {
		return errors.New("invalid module artifact")
	}
	l := m.lock(a.Name)
	l.Lock()
	defer l.Unlock()

	tmp := filepath.Join(m.dir, ".tmp-"+a.Name)
	os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	bin := filepath.Join(tmp, exeName(a.Name))
	logf("downloading %s %s", a.Name, a.Version)
	if err := fetchVerified(ctx, cl, pub, a, bin); err != nil {
		return err
	}
	logf("signature and checksum verified")
	man, err := readManifest(ctx, bin)
	if err != nil {
		return err
	}
	if man.Name != a.Name {
		return fmt.Errorf("module reports name %q, expected %q", man.Name, a.Name)
	}
	if err := saveJSON(filepath.Join(tmp, "manifest.json"), man, 0o600); err != nil {
		return err
	}
	final := filepath.Join(m.dir, a.Name)
	old := filepath.Join(m.dir, ".old-"+a.Name)
	os.RemoveAll(old)
	if _, err := os.Stat(final); err == nil {
		if err := os.Rename(final, old); err != nil {
			return err
		}
	}
	if err := os.Rename(tmp, final); err != nil {
		os.Rename(old, final)
		return err
	}
	os.RemoveAll(old)
	logf("installed %s %s", man.Name, man.Version)
	return nil
}

func (m *moduleManager) remove(name string) error {
	if !moduleNameRe.MatchString(name) {
		return errors.New("invalid module name")
	}
	l := m.lock(name)
	l.Lock()
	defer l.Unlock()
	return os.RemoveAll(filepath.Join(m.dir, name))
}

// call runs a module action and returns its result data.
func (m *moduleManager) call(ctx context.Context, name, action string, params json.RawMessage, timeout time.Duration, logf func(string)) (json.RawMessage, error) {
	if !moduleNameRe.MatchString(name) {
		return nil, errors.New("invalid module name")
	}
	bin := m.binPath(name)
	if _, err := os.Stat(bin); err != nil {
		return nil, fmt.Errorf("module %q is not installed", name)
	}
	// One action per module at a time (package managers hold locks anyway).
	l := m.lock(name)
	l.Lock()
	defer l.Unlock()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "run", action)
	cmd.Dir = filepath.Dir(bin)
	cmd.Env = append(os.Environ(), "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	if len(params) == 0 {
		params = json.RawMessage("{}")
	}
	cmd.Stdin = bytes.NewReader(params)
	cmd.WaitDelay = 10 * time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(stderr)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			logf(sc.Text())
		}
	}()
	var result *proto.ModuleEvent
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64*1024), 32*1024*1024)
	for sc.Scan() {
		var ev proto.ModuleEvent
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			logf(sc.Text())
			continue
		}
		switch ev.T {
		case "log":
			logf(ev.D)
		case "result":
			e := ev
			result = &e
		}
	}
	wg.Wait()
	werr := cmd.Wait()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("action timed out after %s", timeout)
	}
	if result == nil {
		if werr != nil {
			return nil, fmt.Errorf("module exited without result: %v", werr)
		}
		return nil, errors.New("module exited without result")
	}
	if !result.OK {
		return result.Data, errors.New(result.Error)
	}
	return result.Data, nil
}
