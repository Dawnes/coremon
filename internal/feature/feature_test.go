package feature

import "testing"

func TestCollapseCR(t *testing.T) {
	cases := map[string]string{
		"plain": "plain",
		"(Reading database ... 5%\r(Reading database ... 100%\r(Reading database ... 19148 files)": "(Reading database ... 19148 files)",
		"trailing\r": "trailing",
		"a\rb\r":     "b",
	}
	for in, want := range cases {
		if got := CollapseCR(in); got != want {
			t.Errorf("CollapseCR(%q) = %q, want %q", in, got, want)
		}
	}
}
