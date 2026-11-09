package updates

import "testing"

const sample = `NOTE: This is only a simulation!
Reading package lists...
Building dependency tree...
Calculating upgrade...
The following packages will be upgraded:
  base-files libssl3 openssl
Inst base-files [12.4+deb12u5] (12.4+deb12u6 Debian:12.6/stable [amd64])
Inst libssl3 [3.0.11-1~deb12u2] (3.0.13-1~deb12u1 Debian:12.6/stable, Debian-Security:12/stable-security [amd64]) []
Inst linux-image-6.1.0-23-amd64 (6.1.99-1 Debian:12.6/stable [amd64])
Inst tzdata [2024a-0+deb12u1] (2024a-0+deb12u2 Debian:12.6/stable [all])
Inst libc6:i386 [2.35-0ubuntu3.6] (2.35-0ubuntu3.7 Ubuntu:22.04/jammy-updates, Ubuntu:22.04/jammy-security [i386])
Remv oldpkg [1.0]
Conf base-files (12.4+deb12u6 Debian:12.6/stable [amd64])
`

func TestParseAptSim(t *testing.T) {
	s := parseAptSim(sample)
	if len(s.Packages) != 5 {
		t.Fatalf("want 5 packages, got %d: %+v", len(s.Packages), s.Packages)
	}
	byName := map[string]Package{}
	for _, p := range s.Packages {
		byName[p.Name] = p
	}
	ssl := byName["libssl3"]
	if !ssl.Security || ssl.Current != "3.0.11-1~deb12u2" || ssl.Available != "3.0.13-1~deb12u1" || ssl.Arch != "amd64" {
		t.Errorf("libssl3 parsed wrong: %+v", ssl)
	}
	if k := byName["linux-image-6.1.0-23-amd64"]; !k.New || k.Current != "" {
		t.Errorf("kernel should be new: %+v", k)
	}
	if bf := byName["base-files"]; bf.Security || bf.Origin != "Debian:12.6/stable" {
		t.Errorf("base-files parsed wrong: %+v", bf)
	}
	if c := byName["libc6:i386"]; !c.Security || c.Arch != "i386" {
		t.Errorf("libc6:i386 parsed wrong: %+v", c)
	}
	if len(s.Removals) != 1 || s.Removals[0] != "oldpkg" {
		t.Errorf("removals: %v", s.Removals)
	}
	if !s.Packages[0].Security {
		t.Errorf("security updates should sort first")
	}
}

func TestVersionLess(t *testing.T) {
	if !versionLess("6.1.0-9-amd64", "6.1.0-23-amd64") {
		t.Error("6.1.0-9 < 6.1.0-23")
	}
	if versionLess("6.8.12-4-pve", "6.8.12-2-pve") {
		t.Error("6.8.12-4 > 6.8.12-2")
	}
}

func TestPkgNameRe(t *testing.T) {
	for _, ok := range []string{"libssl3", "libc6:i386", "g++-12", "linux-image-6.1.0-23-amd64"} {
		if !PackageNameRe.MatchString(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"-o", "--allow-unauthenticated", "foo bar", "a;rm", "Foo", ""} {
		if PackageNameRe.MatchString(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
}
