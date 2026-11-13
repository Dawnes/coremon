// Package release defines the signed version index (versions.json).
package release

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"

	"coremon/internal/sign"
)

const KeepAgents = 3

const keepMasters = 3

const (
	IndexFile    = "versions.json"
	IndexSigFile = "versions.json.sig"
	IndexTag     = "index"
)

var VersionRe = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][A-Za-z0-9.]+)?$`)

var PlatRe = regexp.MustCompile(`^([a-z0-9]+)-([a-z0-9]+)$`)

type Index struct {
	Format    int       `json:"format"`
	Generated string    `json:"generated"`
	Master    []Release `json:"master"`
	Agent     []Release `json:"agent"`
}

type Release struct {
	Version   string           `json:"version"`
	Published string           `json:"published"`
	Assets    map[string]Asset `json:"assets"`
}

type Asset struct {
	URL       string `json:"url"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
	Signature string `json:"signature"`
}

func TrackInfo(track string) (kind, name string, err error) {
	switch track {
	case "agent":
		return "core", "coremon-agent", nil
	case "master":
		return "master", "coremon-master", nil
	}
	return "", "", fmt.Errorf("unknown track %q", track)
}

func indexMessage(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sign.Message("index", "coremon-versions", "1", "any", "any", hex.EncodeToString(sum[:]))
}

func SignIndex(priv ed25519.PrivateKey, b []byte) string { return sign.Sign(priv, indexMessage(b)) }

func VerifyIndex(pub ed25519.PublicKey, b []byte, sig string) (*Index, error) {
	if !sign.Verify(pub, indexMessage(b), sig) {
		return nil, errors.New("version index signature does not verify")
	}
	var idx Index
	if err := json.Unmarshal(b, &idx); err != nil {
		return nil, fmt.Errorf("version index: %w", err)
	}
	if idx.Format != 1 {
		return nil, fmt.Errorf("version index format %d is not supported", idx.Format)
	}
	return &idx, nil
}

func (idx *Index) Add(track string, r Release) error {
	if !VersionRe.MatchString(r.Version) {
		return fmt.Errorf("invalid version %q", r.Version)
	}
	list := &idx.Agent
	keep := KeepAgents
	if track == "master" {
		list, keep = &idx.Master, keepMasters
	} else if track != "agent" {
		return fmt.Errorf("unknown track %q", track)
	}
	out := []Release{r}
	for _, e := range *list {
		if e.Version != r.Version {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return VersionLess(out[j].Version, out[i].Version) })
	if len(out) > keep {
		out = out[:keep]
	}
	*list = out
	idx.Format = 1
	return nil
}

var verSegRe = regexp.MustCompile(`\d+|[^\d]+`)

func VersionLess(a, b string) bool {
	pa, pb := verSegRe.FindAllString(a, -1), verSegRe.FindAllString(b, -1)
	for i := 0; i < len(pa) && i < len(pb); i++ {
		if pa[i] == pb[i] {
			continue
		}
		na, ea := atoi(pa[i])
		nb, eb := atoi(pb[i])
		if ea && eb {
			return na < nb
		}
		return pa[i] < pb[i]
	}
	return len(pa) < len(pb)
}

func atoi(s string) (int, bool) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	return n, true
}
