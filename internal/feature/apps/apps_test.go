package apps

import (
	"strings"
	"testing"
)

func TestReadable(t *testing.T) {
	for in, want := range map[string]string{
		"1:10.11.6-0+deb12u1":           "10.11.6",
		"8.2.26-1~deb12u1":              "8.2.26",
		"1:9.2p1-2+deb12u3":             "9.2p1",
		"5:27.3.1-1~debian.12~bookworm": "27.3.1",
		"18.19.0+dfsg-6~deb12u2":        "18.19.0",
		"2.4.62-1~deb12u2":              "2.4.62",
		"1.0":                           "1.0",
	} {
		if got := Readable(in); got != want {
			t.Errorf("Readable(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestScan(t *testing.T) {
	rows := scan(strings.NewReader(fakeStatus), fakeSystem(), nil)
	got := map[string]App{}
	var order []string
	for _, a := range rows {
		got[a.Name] = a
		order = append(order, a.Name)
	}
	// One row per version, a-z.
	if !got["PHP 7.4"].Installed || got["PHP 8.2"].Version != "8.2.26" || got["PHP 8.2"].Full != "8.2.26-1~deb12u1" {
		t.Fatalf("PHP rows: %+v %+v", got["PHP 7.4"], got["PHP 8.2"])
	}
	if got["SSH server"].Version != "9.2p1" || got["MariaDB/MySQL"].Version != "10.11.6" || got["Python 3.11"].Version != "3.11.2" || got["OpenSSL"].Version != "3.0.15" {
		t.Fatalf("versions: %+v", got)
	}
	// Removed but not purged (config files only) is not installed.
	if got["Nginx"].Installed || got["Docker"].Installed {
		t.Fatalf("not installed shown as installed: %+v %+v", got["Nginx"], got["Docker"])
	}
	if strings.Join(order[:4], ",") != "Apache,BIND9,Certbot,Docker" {
		t.Fatalf("order: %v", order)
	}
	i74, i82 := -1, -1
	for i, n := range order {
		if n == "PHP 7.4" {
			i74 = i
		}
		if n == "PHP 8.2" {
			i82 = i
		}
	}
	if i74 < 0 || i82 != i74+1 {
		t.Fatalf("PHP versions not together in order: %v", order)
	}
	// All 16 applications appear (installed or not detected).
	names := map[string]bool{}
	for _, a := range rows {
		names[strings.SplitN(a.Name, " ", 2)[0]] = true
	}
	if len(scan(strings.NewReader(""), nil, nil)) != 21 {
		t.Fatalf("empty system: %d rows, want 21", len(scan(strings.NewReader(""), nil, nil)))
	}
}
