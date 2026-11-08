package master

// Master self-update. The master service runs unprivileged and cannot
// replace its own binary. It writes the wanted version to
// <data>/update/request; a root systemd path unit then runs
// `coremon-master apply-update`, which verifies the signature against the
// root-owned public key before installing. A compromised master process can
// therefore only ask for a version that you signed.

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
	if s.repo.find("master", "coremon-master", v, runtime.GOOS, runtime.GOARCH) == nil {
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
	s.audit.write(actorAdmin, clientIP(r), "master.update", v, "from "+s.cfg.Version)
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

// ApplyUpdate runs as root (from coremon-master-update.service).
func ApplyUpdate(dataDir, keyPath, target string) error {
	reqPath := updateRequestPath(dataDir)
	b, err := readLimited(reqPath, 64)
	os.Remove(reqPath) // never loop on the same request
	if err != nil {
		return fmt.Errorf("read request: %w", err)
	}
	version := strings.TrimSpace(string(b))
	if !versionRe.MatchString(version) {
		return errors.New("invalid version in update request")
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
	return exec.Command("systemctl", "restart", "coremon-master.service").Run()
}
