package master

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"coremon/internal/release"
	"coremon/internal/sign"
)

type fakeGitHub struct {
	files map[string][]byte
	srv   *httptest.Server
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	f := &fakeGitHub{files: map[string][]byte{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if b, ok := f.files[r.URL.Path]; ok {
			w.Write(b)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// asset publishes a signed binary and returns its index entry.
func (f *fakeGitHub) asset(priv ed25519.PrivateKey, kind, version, plat string, body []byte) release.Asset {
	goos, arch, _ := strings.Cut(plat, "-")
	sum := sha256.Sum256(body)
	hexSum := hex.EncodeToString(sum[:])
	path := "/dl/" + kind + "-" + version + "-" + plat
	f.files[path] = body
	return release.Asset{URL: f.srv.URL + path, Size: int64(len(body)), SHA256: hexSum,
		Signature: sign.Sign(priv, sign.Message(kind, binName(kind), version, goos, arch, hexSum))}
}

func (f *fakeGitHub) publish(t *testing.T, priv ed25519.PrivateKey, idx release.Index) {
	b, _ := json.Marshal(idx)
	f.files["/versions.json"] = b
	f.files["/versions.json.sig"] = []byte(release.SignIndex(priv, b))
}

func testServer(t *testing.T, pub, indexURL, version string) *Server {
	t.Helper()
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "signing.pub")
	os.WriteFile(keyPath, []byte(pub), 0o644)
	s, err := New(Config{DataDir: filepath.Join(dir, "data"), SigningKeyPath: keyPath, Version: version, IndexURL: indexURL})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func keys(t *testing.T) (string, ed25519.PrivateKey) {
	pub, priv, _ := sign.GenerateKey()
	k, _ := sign.ParsePrivate(priv)
	return pub, k
}

func TestIndexSync(t *testing.T) {
	pub, priv := keys(t)
	gh := newFakeGitHub(t)
	s := testServer(t, pub, gh.srv.URL+"/versions.json", "0.2.0")
	plat := runtime.GOOS + "-" + runtime.GOARCH

	// A legacy agent from before 0.2 must be cleaned up.
	legacy := s.repo.path("core", "0.1.0", "linux-amd64")
	os.MkdirAll(filepath.Dir(legacy), 0o755)
	os.WriteFile(legacy, []byte("old"), 0o755)
	s.cleanupLegacy()
	if _, err := os.Stat(legacy); err == nil {
		t.Fatal("legacy agent 0.1.0 not removed")
	}

	var idx release.Index
	for _, v := range []string{"0.2.0", "0.2.1", "0.3.0", "0.4.0"} {
		idx.Add("agent", release.Release{Version: v, Assets: map[string]release.Asset{
			"linux-amd64": gh.asset(priv, "core", v, "linux-amd64", []byte("agent "+v)),
		}})
	}
	idx.Add("master", release.Release{Version: "0.3.0", Assets: map[string]release.Asset{plat: gh.asset(priv, "master", "0.3.0", plat, []byte("master"))}})
	gh.publish(t, priv, idx)

	if err := s.syncIndex(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	got := s.repo.coreVersions()
	if strings.Join(got, ",") != "0.4.0,0.3.0,0.2.1" {
		t.Fatalf("agent versions = %v, want the newest three", got)
	}
	if s.repo.masterLatest() != "0.3.0" {
		t.Fatalf("newer master not downloaded: %q", s.repo.masterLatest())
	}

	// A new release pushes the oldest out of the list and off the disk.
	idx.Add("agent", release.Release{Version: "0.5.0", Assets: map[string]release.Asset{
		"linux-amd64": gh.asset(priv, "core", "0.5.0", "linux-amd64", []byte("agent 0.5.0")),
	}})
	gh.publish(t, priv, idx)
	if err := s.syncIndex(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := strings.Join(s.repo.coreVersions(), ","); got != "0.5.0,0.4.0,0.3.0" {
		t.Fatalf("after new release: %v", got)
	}
	if _, err := os.Stat(filepath.Dir(s.repo.path("core", "0.2.1", "linux-amd64"))); err == nil {
		t.Fatal("0.2.1 should have been removed from disk")
	}

	// An index signed with another key is rejected and changes nothing.
	_, other := keys(t)
	gh.publish(t, other, idx)
	if err := s.syncIndex(); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("index with wrong key must be rejected, got %v", err)
	}
	if got := strings.Join(s.repo.coreVersions(), ","); got != "0.5.0,0.4.0,0.3.0" {
		t.Fatalf("rejected index changed the store: %v", got)
	}

	// A correctly signed index whose binary was swapped is rejected.
	idx2 := idx
	idx2.Agent = append([]release.Release(nil), idx.Agent...)
	bad := gh.asset(priv, "core", "0.6.0", "linux-amd64", []byte("good"))
	gh.files[strings.TrimPrefix(bad.URL, gh.srv.URL)] = []byte("evil")
	idx2.Add("agent", release.Release{Version: "0.6.0", Assets: map[string]release.Asset{"linux-amd64": bad}})
	gh.publish(t, priv, idx2)
	if err := s.syncIndex(); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("swapped binary must be rejected, got %v", err)
	}
	if s.repo.find("core", "0.6.0", "linux", "amd64") != nil {
		t.Fatal("swapped binary reached the store")
	}
}

func TestMasterNeverDowngrades(t *testing.T) {
	pub, priv := keys(t)
	gh := newFakeGitHub(t)
	s := testServer(t, pub, gh.srv.URL+"/versions.json", "0.5.0")
	plat := runtime.GOOS + "-" + runtime.GOARCH
	var idx release.Index
	idx.Add("master", release.Release{Version: "0.4.0", Assets: map[string]release.Asset{plat: gh.asset(priv, "master", "0.4.0", plat, []byte("old master"))}})
	gh.publish(t, priv, idx)
	s.syncIndex()
	if v := s.repo.masterLatest(); v != "" {
		t.Fatalf("older master %s must not be offered", v)
	}

	// apply-update refuses an older version even if one is on disk.
	data := t.TempDir()
	keyPath := filepath.Join(t.TempDir(), "signing.pub")
	os.WriteFile(keyPath, []byte(pub), 0o644)
	os.MkdirAll(filepath.Join(data, "update"), 0o700)
	os.WriteFile(updateRequestPath(data), []byte("0.4.0\n"), 0o600)
	err := ApplyUpdate(data, keyPath, filepath.Join(t.TempDir(), "coremon-master"), "0.5.0")
	if err == nil || !strings.Contains(err.Error(), "never downgraded") {
		t.Fatalf("expected downgrade refusal, got %v", err)
	}
}

func TestApplyUpdateRejectsBadSignature(t *testing.T) {
	pub, _ := keys(t)
	_, other := keys(t)
	data := t.TempDir()
	keyPath := filepath.Join(t.TempDir(), "signing.pub")
	os.WriteFile(keyPath, []byte(pub), 0o644)
	os.MkdirAll(filepath.Join(data, "update"), 0o700)
	os.WriteFile(updateRequestPath(data), []byte("9.9.9\n"), 0o600)
	plat := goosArch()
	goos, arch, _ := strings.Cut(plat, "-")
	bin := filepath.Join(data, "release", "master", "9.9.9", plat, "coremon-master")
	os.MkdirAll(filepath.Dir(bin), 0o755)
	os.WriteFile(bin, []byte("evil"), 0o755)
	sum, _ := sign.FileSHA256(bin)
	os.WriteFile(bin+".sig", []byte(sign.Sign(other, sign.Message("master", "coremon-master", "9.9.9", goos, arch, sum))), 0o644)
	target := filepath.Join(t.TempDir(), "coremon-master")
	err := ApplyUpdate(data, keyPath, target, "0.2.0")
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

func TestGroupsAndRetention(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.2.0")
	st := s.st
	g1, g2 := &Group{ID: "g_1", Name: "Prod"}, &Group{ID: "g_2", Name: "Lab"}
	st.st.Groups = []*Group{g1, g2}
	for _, id := range []string{"a_1", "a_2", "a_3"} {
		st.st.Agents[id] = &Agent{ID: id, Name: id}
	}
	st.st.Agents["a_1"].GroupID, st.st.Agents["a_1"].Order = "g_2", 0
	st.st.Agents["a_2"].GroupID, st.st.Agents["a_2"].Order = "g_1", 0
	st.st.Agents["a_3"].Order = 0
	var names []string
	for _, a := range s.orderedAgents() {
		names = append(names, a.ID)
	}
	if strings.Join(names, ",") != "a_2,a_1,a_3" {
		t.Fatalf("order = %v (groups in order, ungrouped last)", names)
	}

	// Retention: old finished jobs and audit entries go, recent ones stay.
	now := time.Now().Unix()
	old, recent := now-5*86400, now-3600
	st.st.Jobs = []*Job{
		{ID: "j_old", Status: JobSuccess, Created: old, Finished: old},
		{ID: "j_new", Status: JobSuccess, Created: recent, Finished: recent},
		{ID: "j_open", Status: JobQueued, Created: old},
	}
	for _, j := range st.st.Jobs {
		os.WriteFile(st.jobLogPath(j.ID), []byte("log"), 0o600)
	}
	os.WriteFile(st.jobLogPath("j_orphan"), []byte("log"), 0o600)
	lines := ""
	for _, e := range []auditEntry{{Time: old, Action: "old"}, {Time: recent, Action: "new", Agent: "a_1"}} {
		b, _ := json.Marshal(e)
		lines += string(b) + "\n"
	}
	os.WriteFile(s.audit.path, []byte(lines), 0o600)

	s.purge() // default retention: 3 days
	var ids []string
	for _, j := range st.st.Jobs {
		ids = append(ids, j.ID)
	}
	if strings.Join(ids, ",") != "j_new,j_open" {
		t.Fatalf("jobs after purge: %v", ids)
	}
	for id, want := range map[string]bool{"j_old": false, "j_orphan": false, "j_new": true} {
		if _, err := os.Stat(st.jobLogPath(id)); (err == nil) != want {
			t.Errorf("log %s present=%v, want %v", id, err == nil, want)
		}
	}
	all := s.audit.list("", "", 100)
	if len(all) != 1 || all[0].Action != "new" {
		t.Fatalf("audit after purge: %+v", all)
	}
	if n := len(s.audit.list("a_2", "", 100)); n != 0 {
		t.Fatalf("filter for a_2 returned %d entries", n)
	}
	if n := len(s.audit.list("a_1", "", 100)); n != 1 {
		t.Fatalf("filter for a_1 returned %d entries", n)
	}
}
