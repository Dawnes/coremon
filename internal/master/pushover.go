package master

// The Pushover client (pushover.net): messages, and checking an
// application token and user key.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// pushoverURL: the Pushover API; tests replace it.
var pushoverURL = "https://api.pushover.net/1"

// pushoverPrio: our 1 (min) … 5 (urgent) as Pushover's -2 … 1. Urgent is
// "high" too: no emergency messages that repeat until acknowledged.
func pushoverPrio(p int) string {
	switch p {
	case 1:
		return "-2"
	case 2:
		return "-1"
	case 4, 5:
		return "1"
	}
	return "0"
}

func clipRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// pushoverPost posts a form to the API. permanent: Pushover refused it
// (wrong token, key or device), retrying will not help.
func pushoverPost(path string, form url.Values) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pushoverURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return true, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "coremon")
	resp, err := ntfyClient.Do(req)
	if err != nil {
		return false, cleanNetErr(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	var r struct {
		Status int      `json:"status"`
		Errors []string `json:"errors"`
	}
	json.Unmarshal(raw, &r)
	if resp.StatusCode/100 == 2 && r.Status == 1 {
		return false, nil
	}
	msg := resp.Status
	if len(r.Errors) > 0 {
		msg = strings.Join(r.Errors, "; ")
	}
	perm := resp.StatusCode/100 == 4 && resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode != http.StatusRequestTimeout
	return perm, errors.New("Pushover answered: " + msg)
}

func pushoverSend(t *NotifyTarget, m notifyMsg) (bool, error) {
	f := url.Values{}
	f.Set("token", t.AppToken)
	f.Set("user", t.UserKey)
	f.Set("title", clipRunes(m.Title, 250))
	body := m.Body
	if body == "" {
		body = m.Title // Pushover needs a message
	}
	f.Set("message", clipRunes(body, 1024))
	f.Set("priority", pushoverPrio(m.Prio))
	if t.Devices != "" {
		f.Set("device", t.Devices)
	}
	if m.Click != "" && !t.NoLink {
		f.Set("url", m.Click)
		f.Set("url_title", "Open in Coremon")
	}
	f.Set("timestamp", fmt.Sprint(m.At))
	if m.At == 0 {
		f.Del("timestamp")
	}
	return pushoverPost("/messages.json", f)
}

// pushoverValidate checks the token and user key (and the devices).
// reached: Pushover answered; when false, err says why it could not be
// asked and the settings can still be saved.
func pushoverValidate(t *NotifyTarget) (reached bool, err error) {
	f := url.Values{}
	f.Set("token", t.AppToken)
	f.Set("user", t.UserKey)
	if t.Devices != "" && !strings.Contains(t.Devices, ",") {
		f.Set("device", t.Devices)
	}
	perm, err := pushoverPost("/users/validate.json", f)
	if err != nil && !perm {
		return false, err
	}
	return true, err
}

// checkDevices: "phone, tablet" → "phone,tablet".
func checkDevices(s string) (string, error) {
	var out []string
	for _, d := range strings.Split(s, ",") {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		if !pushDevRe.MatchString(d) {
			return "", fmt.Errorf("device names have up to 25 letters, digits, - and _")
		}
		out = append(out, d)
	}
	if len(out) > 20 {
		return "", errors.New("at most 20 devices")
	}
	return strings.Join(out, ","), nil
}
