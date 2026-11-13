package master

// Master self-update through a root path unit that verifies the signature.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"coremon/internal/release"
	"coremon/internal/sign"
)

func updateRequestPath(dataDir string) string { return filepath.Join(dataDir, "update", "request") }

func goosArch() string { return runtime.GOOS + "-" + runtime.GOARCH }

func (s *Server) handleMasterUpdate(w http.ResponseWriter, r *http.Request) {
	v := s.repo.masterLatest()
	if v == "" || !versionLess(s.cfg.Version, v) {
		http.Error(w, "no newer master version in the repository", http.StatusBadRequest)
		return
	}
	if s.repo.find("master", v, runtime.GOOS, runtime.GOARCH) == nil {
		http.Error(w, "no master build for this platform", http.StatusBadRequest)
		return
	}
	p := updateRequestPath(s.cfg.DataDir)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(p, []byte(v+"\n"), 0o600); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.audit.write(actor(r), clientIP(r), "master.update", v, "from "+s.cfg.Version)
	go func() {
		time.Sleep(20 * time.Second)
		if _, err := os.Stat(p); err == nil {
			log.Printf("master update request not picked up; is coremon-master-update.path enabled?")
		}
	}()
	writeJSON(w, map[string]string{"requested": v})
}

func readLimited(p string, max int64) ([]byte, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, errors.New("file too large")
	}
	return b, nil
}

func ApplyUpdate(dataDir, keyPath, target, current string) error {
	reqPath := updateRequestPath(dataDir)
	b, err := readLimited(reqPath, 64)
	os.Remove(reqPath)
	if err != nil {
		return fmt.Errorf("read request: %w", err)
	}
	version := strings.TrimSpace(string(b))
	if !release.VersionRe.MatchString(version) {
		return errors.New("invalid version in update request")
	}
	if !versionLess(current, version) {
		return fmt.Errorf("refusing %s: not newer than the installed master %s (the master is never downgraded)", version, current)
	}
	kb, err := readLimited(keyPath, 1024)
	if err != nil {
		return fmt.Errorf("signing key: %w", err)
	}
	pub, err := sign.ParsePublic(string(kb))
	if err != nil {
		return err
	}
	dir := filepath.Join(dataDir, "release", "master", version, runtime.GOOS+"-"+runtime.GOARCH)
	bin, err := readLimited(filepath.Join(dir, "coremon-master"), 256<<20)
	if err != nil {
		return fmt.Errorf("read binary: %w", err)
	}
	sig, err := readLimited(filepath.Join(dir, "coremon-master.sig"), 1024)
	if err != nil {
		return fmt.Errorf("read signature: %w", err)
	}
	sum := sha256.Sum256(bin)
	msg := sign.Message("master", "coremon-master", version, runtime.GOOS, runtime.GOARCH, hex.EncodeToString(sum[:]))
	if !sign.Verify(pub, msg, string(sig)) {
		return errors.New("signature verification FAILED - not installing")
	}
	tmp := target + ".new"
	if err := os.WriteFile(tmp, bin, 0o755); err != nil {
		return err
	}
	if out, err := exec.Command(tmp, "version").Output(); err != nil || !bytes.Contains(out, []byte(version)) {
		os.Remove(tmp)
		return fmt.Errorf("new binary did not report version %s", version)
	}
	if err := os.Rename(tmp, target); err != nil {
		return err
	}
	log.Printf("installed coremon-master %s; restarting service", version)
	EnsureResetLink(target)
	return exec.Command("systemctl", "restart", "coremon-master.service").Run()
}

const ResetLink = "/usr/local/sbin/coremon-reset"

func EnsureResetLink(target string) {
	if cur, err := os.Readlink(ResetLink); err == nil && cur == target {
		return
	}
	os.Remove(ResetLink)
	if err := os.Symlink(target, ResetLink); err != nil {
		log.Printf("coremon-reset link: %v", err)
	}
}
