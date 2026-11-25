package updates

import "testing"

const testZypperLU = `<?xml version='1.0'?>
<stream>
<message type="info">Loading repository data...</message>
<update-status version="0.6">
<update-list>
<update kind="package" name="curl" edition="8.6.0-150600.4.9.1" arch="x86_64" edition-old="8.6.0-150600.4.6.1">
<summary>A Tool for Transferring Data from URLs</summary>
<source url="https://download.opensuse.org/update/leap/15.6/sle" alias="repo-sle-update"/>
</update>
<update kind="package" name="libopenssl3" edition="3.1.4-150600.5.21.1" arch="x86_64">
<source url="https://download.opensuse.org/update/leap/15.6/sle" alias="repo-sle-update"/>
</update>
<update kind="package" name="vim" edition="9.1.0836-150500.20.15.1" arch="x86_64" edition-old="9.1.0330-150500.20.9.1">
<source url="https://download.opensuse.org/update/leap/15.6/oss" alias="repo-oss-update"/>
</update>
<update kind="patch" name="openSUSE-SLE-15.6-2024-4000" edition="1" arch="noarch" category="security"/>
</update-list>
</update-status>
</stream>`

// Real output of openSUSE Leap 16.0 (shortened).
const testZypperLP = `<?xml version='1.0'?>
<stream>
<message type="info">Loading repository data...</message>
<update-status version="0.6">
<update-list>
<update kind="patch" name="openSUSE-Leap-16.0-961" edition="1" arch="noarch" status="needed" category="security" severity="moderate" pkgmanager="true" restart="false" interactive="false">
<summary>Security update for zypper</summary>
<source url="http://cdn.opensuse.org/distribution/leap/16.0/repo/oss/aarch64" alias="openSUSE:repo-oss"/>
</update>
<update kind="patch" name="openSUSE-Leap-16.0-1151" edition="1" arch="noarch" status="needed" category="security" severity="important" pkgmanager="false">
<source alias="openSUSE:repo-oss"/>
</update>
<update kind="patch" name="openSUSE-Leap-16.0-900" edition="1" arch="noarch" status="applied" category="security"/>
<update kind="patch" name="openSUSE-Leap-16.0-901" edition="1" arch="noarch" status="needed" category="recommended"/>
</update-list>
</update-status>
</stream>`

const testZypperInfo = `Loading repository data...
Reading installed packages...


Information for patch openSUSE-Leap-16.0-1151:
----------------------------------------------
Repository  : repo-oss (16.0)
Name        : openSUSE-Leap-16.0-1151
Status      : needed
Category    : security
Summary     : Security update for curl and openssl
Description :
    This update for curl fixes:
    curl < 8.6.0 was vulnerable.
Conflicts   : [5]
    curl.x86_64 < 8.6.0-150600.4.9.1
    curl.noarch < 8.6.0-150600.4.9.1
    libopenssl3.aarch64 < 3.1.4-150600.5.21.1
    libopenssl3.x86_64 < 3.1.4-150600.5.21.1
    python3.13-foo.noarch < 1.0-1.1
    vim-data < 9.1.0330-150500.20.9.1
`

const testZypperDup = `<?xml version='1.0'?>
<stream>
<install-summary>
<to-upgrade>
<solvable type="package" name="kernel-default" edition="6.12.1-1.1" arch="x86_64" edition-old="6.11.8-1.1" repository="repo-oss"/>
</to-upgrade>
<to-install>
<solvable type="package" name="libnew1" edition="1.0-1.1" arch="x86_64" repository="repo-oss"/>
</to-install>
<to-remove>
<solvable type="package" name="libold0" edition="0.9-1.1" arch="x86_64"/>
</to-remove>
</install-summary>
</stream>`

func TestParseZypper(t *testing.T) {
	inst := "curl|x86_64|0:8.6.0-150600.4.6.1\nlibopenssl3|x86_64|0:3.1.4-150600.5.15.1\nvim|x86_64|0:9.1.0330-150500.20.9.1\n"
	if n := neededPatches(testZypperLP); len(n) != 2 || n[0] != "openSUSE-Leap-16.0-961" || n[1] != "openSUSE-Leap-16.0-1151" {
		t.Errorf("needed: %v", n)
	}
	if f := patchFixes(testZypperInfo); len(f) != 6 || f["curl"] != "8.6.0-150600.4.9.1" || f["python3.13-foo"] == "" ||
		f["vim-data"] == "" || f["libopenssl3.aarch64"] == "" || f["curl.x86_64"] == "" {
		t.Errorf("fixes: %v", f)
	}
	s := parseZypper(inst, testZypperLU, testZypperInfo)
	got := map[string]Package{}
	for _, p := range s.Packages {
		got[p.Name] = p
	}
	if len(s.Packages) != 3 {
		t.Fatalf("packages: %+v", s.Packages)
	}
	if p := got["curl"]; !p.Security || p.Current != "8.6.0-150600.4.6.1" || p.Available != "8.6.0-150600.4.9.1" || p.Origin != "repo-sle-update" || p.New {
		t.Errorf("curl: %+v", p)
	}
	if p := got["libopenssl3"]; !p.Security || p.Current != "3.1.4-150600.5.15.1" { // no edition-old: from rpm
		t.Errorf("libopenssl3: %+v", p)
	}
	if p := got["vim"]; p.Security || p.Origin != "repo-oss-update" {
		t.Errorf("vim: %+v", p)
	}
	// Tumbleweed: a dry run of dup.
	d := parseZypper("kernel-default|x86_64|0:6.11.8-1.1\n", testZypperDup, "")
	if len(d.Packages) != 2 || d.Packages[0].Name != "kernel-default" || d.Packages[0].New || !d.Packages[1].New {
		t.Errorf("dup: %+v", d.Packages)
	}
	if n := zypperRepos(`<stream><repo-list><repo alias="a" enabled="1"/><repo alias="b" enabled="0"/></repo-list></stream>`); n != 1 {
		t.Errorf("repos: %d", n)
	}
	if n := zypperRepos(`<stream><message type="warning">No repositories defined.</message></stream>`); n != 0 {
		t.Errorf("no repos: %d", n)
	}
}

func TestApk(t *testing.T) {
	out := `Installed:                                Available:
busybox-1.36.1-r29                      < 1.36.1-r31
curl-8.11.0-r2                          < 8.11.1-r0
py3-pip-pyc-24.0-r2                     < 24.3.1-r0
openssh-server-common-9.7_p1-r3         < 9.7_p1-r4
`
	s := parseApk(out)
	got := map[string]Package{}
	for _, p := range s.Packages {
		got[p.Name] = p
	}
	if len(s.Packages) != 4 || got["curl"].Current != "8.11.0-r2" || got["curl"].Available != "8.11.1-r0" ||
		got["py3-pip-pyc"].Current != "24.0-r2" || got["openssh-server-common"].Available != "9.7_p1-r4" {
		t.Errorf("apk: %+v", s.Packages)
	}
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"9.7_p1-r3", "9.7_p1-r4", -1},
		{"9.7_p1-r3", "9.7-r3", 1},   // _p after the release
		{"1.0_rc1-r0", "1.0-r0", -1}, // _rc before
		{"1.0_alpha2", "1.0_beta1", -1},
		{"1.2.10-r0", "1.2.9-r5", 1},
		{"1.2a-r0", "1.2-r0", 1},
		{"1.2a-r0", "1.2b-r0", -1},
		{"3.3.2-r0", "3.3.2-r0", 0},
		{"2.0_git20240101-r0", "2.0-r0", 1},
		{"1.0", "1.0-r1", -1},
	} {
		if got := ApkVerCmp(c.a, c.b); got != c.want {
			t.Errorf("%s vs %s: %d, want %d", c.a, c.b, got, c.want)
		}
		if got := ApkVerCmp(c.b, c.a); got != -c.want {
			t.Errorf("%s vs %s: %d, want %d", c.b, c.a, got, -c.want)
		}
	}
	if n, v, ok := SplitApk("openssh-server-9.7_p1-r3"); !ok || n != "openssh-server" || v != "9.7_p1-r3" {
		t.Errorf("split: %s %s", n, v)
	}
	db := "C:Q1abc=\nP:libssl3\nV:3.5.9-r0\no:openssl\n\nP:libcrypto3\nV:3.5.9-r0\no:openssl\n\nP:zlib\nV:1.3.2-r1\n"
	if o := apkOrigins(db); o["libssl3"] != "openssl" || o["libcrypto3"] != "openssl" || o["zlib"] != "" {
		t.Errorf("origins: %v", o)
	}
	for in, want := range map[string]string{"openssl-3-3.5.0-160000.4.1.src.rpm": "openssl-3", "(none)": "", "kernel-default-6.12.1-1.1.nosrc.rpm": "kernel-default"} {
		if got := srpmName(in); got != want {
			t.Errorf("srpmName(%s) = %q", in, got)
		}
	}
}
