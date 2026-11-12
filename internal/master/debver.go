package master

// Debian package version comparison, as dpkg does it
// ([epoch:]upstream[-revision], with "~" sorting before everything, even
// the end of the string). Used for Debian and Ubuntu versions alike.

import "strings"

// debVerCmp returns -1, 0 or 1 as a is older than, equal to or newer than b.
func debVerCmp(a, b string) int {
	ea, ua, ra := debSplit(a)
	eb, ub, rb := debSplit(b)
	if c := numCmp(ea, eb); c != 0 {
		return c
	}
	if c := debPartCmp(ua, ub); c != 0 {
		return c
	}
	return debPartCmp(ra, rb)
}

func debSplit(v string) (epoch, upstream, revision string) {
	v = strings.TrimSpace(v)
	if i := strings.IndexByte(v, ':'); i >= 0 {
		epoch, v = v[:i], v[i+1:]
	}
	if i := strings.LastIndexByte(v, '-'); i >= 0 {
		v, revision = v[:i], v[i+1:]
	}
	return epoch, v, revision
}

// numCmp compares digit strings of any length (leading zeros ignored).
func numCmp(a, b string) int {
	a, b = strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
	if len(a) != len(b) {
		if len(a) < len(b) {
			return -1
		}
		return 1
	}
	return strings.Compare(a, b)
}

// debOrder: "~" first, then the end of the string, then letters, then
// everything else.
func debOrder(c byte) int {
	switch {
	case c == '~':
		return -1
	case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z':
		return int(c)
	default:
		return int(c) + 256
	}
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func debPartCmp(a, b string) int {
	for a != "" || b != "" {
		// the non-digit run
		for (a != "" && !isDigit(a[0])) || (b != "" && !isDigit(b[0])) {
			ca, cb := 0, 0 // 0: the run ended here
			na, nb := a != "" && !isDigit(a[0]), b != "" && !isDigit(b[0])
			if na {
				ca = debOrder(a[0])
			}
			if nb {
				cb = debOrder(b[0])
			}
			if ca != cb {
				if ca < cb {
					return -1
				}
				return 1
			}
			if na {
				a = a[1:]
			}
			if nb {
				b = b[1:]
			}
		}
		// the digit run
		da, db := leadingDigits(a), leadingDigits(b)
		if c := numCmp(da, db); c != 0 {
			return c
		}
		a, b = a[len(da):], b[len(db):]
	}
	return 0
}

func leadingDigits(s string) string {
	i := 0
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	return s[:i]
}
