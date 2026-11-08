package master

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestPurgeAfterChange(t *testing.T) {
	at := func(s string) time.Time { v, _ := time.Parse("15:04:05", s); return v }
	cases := map[string]string{
		"13:10:00": "14:00:00", // plenty of time: next full hour
		"13:56:59": "14:00:00", // 3m01s left: still this hour
		"13:57:01": "15:00:00", // under 3 minutes: the hour after
		"13:59:59": "15:00:00",
	}
	for now, want := range cases {
		if got := purgeAfterChange(at(now)).Format("15:04:05"); got != want {
			t.Errorf("change at %s: cleanup at %s, want %s", now, got, want)
		}
	}
	if got := nextFullHour(at("13:00:00")).Format("15:04:05"); got != "14:00:00" {
		t.Errorf("nextFullHour(13:00) = %s", got)
	}
}

func TestInstallURLUsesAgentPort(t *testing.T) {
	s := &Server{cfg: Config{Listen: ":8443"}}
	for _, host := range []string{"192.168.5.108", "192.168.5.108:443", "coremon.lan:8443"} {
		r := httptest.NewRequest("GET", "https://"+host+"/", nil)
		r.Host = host
		got := s.derivedURL(r)
		want := "https://" + map[string]string{"192.168.5.108": "192.168.5.108", "192.168.5.108:443": "192.168.5.108", "coremon.lan:8443": "coremon.lan"}[host] + ":8443"
		if got != want {
			t.Errorf("website at %s: install URL %s, want %s", host, got, want)
		}
	}
}
