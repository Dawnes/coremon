package apps

// Installed Alpine packages, from apk's database: name (P:), version (V:)
// and source package (o:, "origin").

import (
	"os"
	"strings"
)

const apkDB = "/lib/apk/db/installed"

func parseApkDB(db string) (map[string]pkgInfo, map[string]string) {
	info, origin := map[string]pkgInfo{}, map[string]string{}
	var name, ver, src string
	flush := func() {
		if name != "" && ver != "" {
			p := pkgInfo{ver: ver, src: name, srcVer: ver}
			if src != "" {
				p.src = src
			}
			info[name] = p
			origin[name] = "Alpine"
		}
		name, ver, src = "", "", ""
	}
	for _, line := range strings.Split(db, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "P:"):
			name = line[2:]
		case strings.HasPrefix(line, "V:"):
			ver = line[2:]
		case strings.HasPrefix(line, "o:"):
			src = line[2:]
		}
	}
	flush()
	return info, origin
}

// osID: ID from /etc/os-release.
func osID() string {
	b, _ := os.ReadFile("/etc/os-release")
	for _, line := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(line, "ID="); ok {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}

// suseID: SUSE Linux Enterprise or openSUSE.
func suseID(id string) bool {
	return id == "sles" || id == "sled" || strings.HasPrefix(id, "opensuse") || strings.HasPrefix(id, "sle-")
}
