package master

// Authenticator codes (RFC 6238): HMAC-SHA1, 6 digits, 30 seconds.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const totpPeriod = 30

var totpB32 = base32.StdEncoding.WithPadding(base32.NoPadding)

func newTOTPSecret() string {
	b := make([]byte, 20)
	rand.Read(b)
	return totpB32.EncodeToString(b)
}

func totpCode(secret string, step int64) string {
	key, err := totpB32.DecodeString(strings.ToUpper(secret))
	if err != nil {
		return ""
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	m := hmac.New(sha1.New, key)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", v%1000000)
}

// totpCheck returns the time step the code belongs to (now, or one step
// either side), or -1. Steps up to last were used already.
func totpCheck(secret, code string, now time.Time, last int64) int64 {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != 6 || strings.Trim(code, "0123456789") != "" {
		return -1
	}
	t := now.Unix() / totpPeriod
	for _, s := range []int64{t, t - 1, t + 1} {
		if s > last && hmac.Equal([]byte(totpCode(secret, s)), []byte(code)) {
			return s
		}
	}
	return -1
}

func totpURI(account, secret string) string {
	return "otpauth://totp/" + url.PathEscape("Coremon:"+account) + "?secret=" + secret +
		"&issuer=Coremon&algorithm=SHA1&digits=6&period=30"
}
