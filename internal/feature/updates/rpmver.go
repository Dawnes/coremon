package updates

// RPM version comparison ([epoch:]version-release), as rpm does it
// (rpmvercmp). Used by the agent for kernels and by the master for the
// fixed versions in Rocky Linux and AlmaLinux advisories.

import "strings"

// RPMVerCmp compares two [epoch:]version[-release] strings: -1, 0 or 1.
func RPMVerCmp(a, b string) int {
	ea, va, ra := rpmSplit(a)
	eb, vb, rb := rpmSplit(b)
	if c := rpmNumCmp(ea, eb); c != 0 {
		return c
	}
	if c := rpmSegCmp(va, vb); c != 0 {
		return c
	}
	if ra == "" || rb == "" { // a missing release matches any release
		return 0
	}
	return rpmSegCmp(ra, rb)
}

func rpmSplit(v string) (epoch, version, release string) {
	v = strings.TrimSpace(v)
	epoch = "0"
	if i := strings.IndexByte(v, ':'); i >= 0 {
		epoch, v = v[:i], v[i+1:]
		if epoch == "" {
			epoch = "0"
		}
	}
	if i := strings.LastIndexByte(v, '-'); i >= 0 {
		v, release = v[:i], v[i+1:]
	}
	return epoch, v, release
}

func rpmNumCmp(a, b string) int {
	a, b = strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
	if len(a) != len(b) {
		if len(a) < len(b) {
			return -1
		}
		return 1
	}
	return strings.Compare(a, b)
}

func isAlnum(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// rpmSegCmp: rpmvercmp. Segments of digits or letters are compared one by
// one; digits are newer than letters; "~" sorts before everything (also
// the end), "^" after the end but before anything else.
func rpmSegCmp(a, b string) int {
	if a == b {
		return 0
	}
	for a != "" || b != "" {
		for a != "" && !isAlnum(a[0]) && a[0] != '~' && a[0] != '^' {
			a = a[1:]
		}
		for b != "" && !isAlnum(b[0]) && b[0] != '~' && b[0] != '^' {
			b = b[1:]
		}
		if (a != "" && a[0] == '~') || (b != "" && b[0] == '~') {
			if a == "" || a[0] != '~' {
				return 1
			}
			if b == "" || b[0] != '~' {
				return -1
			}
			a, b = a[1:], b[1:]
			continue
		}
		if (a != "" && a[0] == '^') || (b != "" && b[0] == '^') {
			if a == "" {
				return -1
			}
			if b == "" {
				return 1
			}
			if a[0] != '^' {
				return 1
			}
			if b[0] != '^' {
				return -1
			}
			a, b = a[1:], b[1:]
			continue
		}
		if a == "" || b == "" {
			break
		}
		digit := a[0] >= '0' && a[0] <= '9'
		take := func(s string) (string, string) {
			i := 0
			for i < len(s) && isAlnum(s[i]) && (s[i] >= '0' && s[i] <= '9') == digit {
				i++
			}
			return s[:i], s[i:]
		}
		var sa, sb string
		sa, a = take(a)
		sb, b = take(b)
		if sb == "" { // different kinds: digits are newer
			if digit {
				return 1
			}
			return -1
		}
		if digit {
			if c := rpmNumCmp(sa, sb); c != 0 {
				return c
			}
		} else if c := strings.Compare(sa, sb); c != 0 {
			if c < 0 {
				return -1
			}
			return 1
		}
	}
	switch {
	case a == "" && b == "":
		return 0
	case a == "":
		return -1
	}
	return 1
}
