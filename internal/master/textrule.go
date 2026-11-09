package master

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The text rule for free text: names typed in the website (systems, groups,
// install-command notes) and text an agent reports. Shown everywhere as text
// only, but kept free of anything that could ever act as code or markup.

// shellAndMarkup are characters with a meaning in shells, HTML or scripts.
const shellAndMarkup = "<>\"'`$\\;|&"

// forbiddenRune: control characters (C0, DEL, C1), invisible and
// direction-changing format characters (zero-width, bidi overrides), line and
// paragraph separators, private-use and non-characters, and shellAndMarkup.
func forbiddenRune(r rune) bool {
	switch {
	case r == utf8.RuneError,
		unicode.IsControl(r),
		unicode.Is(unicode.Cf, r), // format: zero-width, bidi controls, soft hyphen...
		unicode.Is(unicode.Zl, r), unicode.Is(unicode.Zp, r),
		unicode.Is(unicode.Co, r),  // private use
		r >= 0xFFF0 && r <= 0xFFFF, // specials, non-characters
		strings.ContainsRune(shellAndMarkup, r):
		return true
	}
	return false
}

// Website names: limits in characters.
const (
	maxSystemName = 64
	maxGroupName  = 48
	maxNoteLen    = 64
)

// checkName validates a name typed in the website: trimmed, at most max
// characters, no forbidden characters. It refuses rather than repairs.
func checkName(what, s string, max int) (string, error) {
	// Check before trimming: trimming would silently drop some control
	// characters at the ends (e.g. U+0085), and the rule is to refuse.
	s = strings.Trim(s, " ")
	if !utf8.ValidString(s) {
		return "", fmt.Errorf("%s contains characters that are not allowed", what)
	}
	for _, r := range s {
		if forbiddenRune(r) {
			if r < 0x20 || r == 0x7f || !unicode.IsPrint(r) {
				return "", fmt.Errorf("%s contains invisible or control characters, which are not allowed", what)
			}
			return "", fmt.Errorf("%s contains %q, which is not allowed (not allowed: %s and control characters)", what, r, spaced(shellAndMarkup))
		}
	}
	if n := utf8.RuneCountInString(s); n > max {
		return "", fmt.Errorf("%s is too long: %d characters, at most %d", what, n, max)
	}
	return s, nil
}

func spaced(s string) string { return strings.Join(strings.Split(s, ""), " ") }

// cleanText repairs text an agent reports: invalid UTF-8 and forbidden
// characters are dropped, spaces trimmed, at most max characters.
func cleanText(s string, max int) string {
	var b strings.Builder
	n := 0
	for _, r := range strings.ToValidUTF8(s, "") {
		if forbiddenRune(r) {
			continue
		}
		if n == max {
			break
		}
		b.WriteRune(r)
		n++
	}
	return strings.TrimSpace(b.String())
}

// cleanLog repairs free-running text such as job output and error messages:
// punctuation stays (it is a log), but control characters other than line
// breaks and tabs, and invisible format characters, are dropped.
func cleanLog(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return r
		case r == utf8.RuneError, unicode.IsControl(r), unicode.Is(unicode.Cf, r),
			unicode.Is(unicode.Zl, r), unicode.Is(unicode.Zp, r):
			return -1
		}
		return r
	}, strings.ToValidUTF8(s, ""))
}
