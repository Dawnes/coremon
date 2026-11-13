package master

// The text rule: free text never contains anything that could act as code or markup.

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The text rule for free text: names typed in the website (systems, groups,
// install-command notes) and text an agent reports. Shown everywhere as text
// only, but kept free of anything that could ever act as code or markup.

const shellAndMarkup = "<>\"'`$\\;|&"

func forbiddenRune(r rune) bool {
	switch {
	case r == utf8.RuneError,
		unicode.IsControl(r),
		unicode.Is(unicode.Cf, r),
		unicode.Is(unicode.Zl, r), unicode.Is(unicode.Zp, r),
		unicode.Is(unicode.Co, r),
		r >= 0xFFF0 && r <= 0xFFFF,
		strings.ContainsRune(shellAndMarkup, r):
		return true
	}
	return false
}

const (
	maxSystemName = 64
	maxGroupName  = 48
	maxNoteLen    = 64
)

func checkName(what, s string, max int) (string, error) {
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
