package apps

// Installed RPM packages (Rocky Linux, AlmaLinux, RHEL): name, version,
// source package and vendor.

import "strings"

const rpmFormat = `%{NAME}\t%{EPOCHNUM}\t%{VERSION}-%{RELEASE}\t%{SOURCERPM}\t%{VENDOR}\n`

// parseRPM: the packages and, per package, where it comes from. The source
// version carries the epoch, as the distributions' advisories write it.
func parseRPM(out string) (map[string]pkgInfo, map[string]string) {
	info, origin := map[string]pkgInfo{}, map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(f) < 5 || f[0] == "" || f[0] == "gpg-pubkey" {
			continue
		}
		if _, dup := info[f[0]]; dup { // several architectures: the first
			continue
		}
		name, epoch, vr := f[0], strings.TrimSpace(f[1]), f[2]
		ver := vr
		if epoch != "" && epoch != "0" {
			ver = epoch + ":" + vr
		}
		p := pkgInfo{ver: ver, src: name, srcVer: ver}
		if sn, sv, ok := splitSRPM(f[3]); ok {
			p.src, p.srcVer = sn, sv
			if epoch != "" && epoch != "0" {
				p.srcVer = epoch + ":" + sv
			}
		}
		info[name] = p
		origin[name] = vendorOrigin(f[4])
	}
	return info, origin
}

// splitSRPM: "openssh-9.9p1-7.el10_3.src.rpm" → "openssh", "9.9p1-7.el10_3".
func splitSRPM(s string) (string, string, bool) {
	s = strings.TrimSuffix(strings.TrimSuffix(s, ".rpm"), ".src")
	s = strings.TrimSuffix(s, ".nosrc")
	r := strings.LastIndexByte(s, '-')
	if r <= 0 {
		return "", "", false
	}
	v := strings.LastIndexByte(s[:r], '-')
	if v <= 0 {
		return "", "", false
	}
	return s[:v], s[v+1:], true
}

// vendorOrigin: the distribution's name for its own packages (as the master
// compares it), otherwise the vendor as given.
func vendorOrigin(v string) string {
	v = strings.TrimSpace(v)
	switch {
	case v == "" || v == "(none)":
		return ""
	case strings.Contains(v, "Rocky"):
		return "Rocky Linux"
	case strings.Contains(v, "AlmaLinux"):
		return "AlmaLinux"
	case strings.Contains(v, "Red Hat"):
		return "Red Hat"
	case strings.Contains(v, "Fedora Project"):
		return "EPEL"
	}
	if len(v) > 64 {
		v = v[:64]
	}
	return v
}
