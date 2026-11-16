package master

import "testing"

func TestNotifyEmailFormat(t *testing.T) {
	for _, ok := range []string{"ops@example.com", "first.last+tag@sub.example.co.uk", "a@b.io"} {
		if !emailRe.MatchString(ok) {
			t.Errorf("refused %q", ok)
		}
	}
	for _, bad := range []string{"", "plain", "@example.com", "a@", "a@b", "a b@example.com", "a@-example.com", "a@example.c", "a@@example.com"} {
		if emailRe.MatchString(bad) {
			t.Errorf("accepted %q", bad)
		}
	}
}
