package master

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"coremon/internal/sign"
)

type file struct {
	name string
	data []byte
}

func tarball(t *testing.T, files []file) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range files {
		tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0o644, Size: int64(len(f.data)), Typeflag: tar.TypeReg})
		tw.Write(f.data)
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func signed(t *testing.T, priv string, kind, name, ver, plat, path string, bin []byte) []file {
	k, _ := sign.ParsePrivate(priv)
	tmp := filepath.Join(t.TempDir(), "b")
	os.WriteFile(tmp, bin, 0o644)
	sum, _ := sign.FileSHA256(tmp)
	goos, arch, _ := strings.Cut(plat, "-")
	sig := sign.Sign(k, sign.Message(kind, name, ver, goos, arch, sum))
	return []file{{path, bin}, {path + ".sig", []byte(sig)}}
}

func newTestServer(t *testing.T, pub string) *Server {
	t.Helper()
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "signing.pub")
	os.WriteFile(keyPath, []byte(pub), 0o644)
	s, err := New(Config{DataDir: filepath.Join(dir, "data"), SigningKeyPath: keyPath, Version: "0.1.0"})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestGitHubImport(t *testing.T) {
	pub, priv, _ := sign.GenerateKey()
	_, otherPriv, _ := sign.GenerateKey()
	p := "coremon-0.2.0/release/"
	good := append(signed(t, priv, "core", "coremon-agent", "0.2.0", "linux-amd64", p+"core/0.2.0/linux-amd64/coremon-agent", []byte("agent")),
		signed(t, priv, "master", "coremon-master", "0.2.0", "linux-amd64", p+"master/0.2.0/linux-amd64/coremon-master", []byte("master"))...)
	good = append(good, signed(t, priv, "module", "updates", "0.2.0", "linux-amd64", p+"modules/updates/0.2.0/linux-amd64/updates", []byte("mod"))...)
	evil := signed(t, otherPriv, "core", "coremon-agent", "0.3.0", "linux-amd64", "coremon-0.3.0/release/core/0.3.0/linux-amd64/coremon-agent", []byte("evil"))
	trav := []file{{"coremon-0.4.0/release/../../../x", []byte("x")}}

	blobs := map[string][]byte{
		"/good.tgz": tarball(t, good),
		"/evil.tgz": tarball(t, evil),
		"/trav.tgz": tarball(t, trav),
	}
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if b, ok := blobs[r.URL.Path]; ok {
			w.Write(b)
			return
		}
		http.NotFound(w, r)
	}))
	defer hs.Close()

	s := newTestServer(t, pub)
	rel := func(v, path string) ghRelease {
		return ghRelease{TagName: "v" + v, Assets: []ghAsset{{Name: "coremon-" + v + ".tar.gz", URL: hs.URL + path}}}
	}
	s.gh.raw["0.2.0"] = rel("0.2.0", "/good.tgz")
	s.gh.raw["0.3.0"] = rel("0.3.0", "/evil.tgz")
	s.gh.raw["0.4.0"] = rel("0.4.0", "/trav.tgz")

	if err := s.githubImport("0.2.0", "test"); err != nil {
		t.Fatalf("good import failed: %v", err)
	}
	if !s.repo.hasCoreVersion("0.2.0") || s.repo.masterLatest() != "0.2.0" || s.repo.moduleLatest("updates") != "0.2.0" {
		t.Fatalf("repo not updated: %+v", s.repo.summary())
	}
	if err := s.githubImport("0.3.0", "test"); err == nil || !strings.Contains(err.Error(), "verification failed") {
		t.Fatalf("wrongly signed release must be rejected, got %v", err)
	}
	if s.repo.hasCoreVersion("0.3.0") {
		t.Fatal("rejected release leaked into repository")
	}
	if err := s.githubImport("0.4.0", "test"); err == nil {
		t.Fatal("path traversal archive must be rejected")
	}
}

func TestApplyUpdateRejectsBadSignature(t *testing.T) {
	pub, _, _ := sign.GenerateKey()
	_, otherPriv, _ := sign.GenerateKey()
	data := t.TempDir()
	keyPath := filepath.Join(t.TempDir(), "signing.pub")
	os.WriteFile(keyPath, []byte(pub), 0o644)
	os.MkdirAll(filepath.Join(data, "update"), 0o700)
	os.WriteFile(updateRequestPath(data), []byte("9.9.9\n"), 0o600)
	plat := goosArch()
	for _, f := range signed(t, otherPriv, "master", "coremon-master", "9.9.9", plat, filepath.Join(data, "release", "master", "9.9.9", plat, "coremon-master"), []byte("evil")) {
		os.MkdirAll(filepath.Dir(f.name), 0o755)
		os.WriteFile(f.name, f.data, 0o644)
	}
	target := filepath.Join(t.TempDir(), "coremon-master")
	err := ApplyUpdate(data, keyPath, target)
	if err == nil || !strings.Contains(err.Error(), "FAILED") {
		t.Fatalf("expected signature failure, got %v", err)
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("binary must not be installed")
	}
	if _, err := os.Stat(updateRequestPath(data)); err == nil {
		t.Fatal("request must be consumed")
	}
}
