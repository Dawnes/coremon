package master

// CVSS 3.x base score from a vector string ("CVSS:3.1/AV:N/AC:L/...").
// Only the base metrics count; temporal and environmental ones are ignored.

import (
	"math"
	"strings"
)

// cvss3Score returns the base score (0.0-10.0) and whether the vector
// could be read.
func cvss3Score(vector string) (float64, bool) {
	parts := strings.Split(vector, "/")
	if len(parts) < 9 || !strings.HasPrefix(parts[0], "CVSS:3.") {
		return 0, false
	}
	m := map[string]string{}
	for _, p := range parts[1:] {
		if k, v, ok := strings.Cut(p, ":"); ok {
			m[k] = v
		}
	}
	scope := m["S"]
	if scope != "U" && scope != "C" {
		return 0, false
	}
	look := func(k string, vals map[string]float64) (float64, bool) {
		v, ok := vals[m[k]]
		return v, ok
	}
	av, ok1 := look("AV", map[string]float64{"N": 0.85, "A": 0.62, "L": 0.55, "P": 0.2})
	ac, ok2 := look("AC", map[string]float64{"L": 0.77, "H": 0.44})
	prVals := map[string]float64{"N": 0.85, "L": 0.62, "H": 0.27}
	if scope == "C" {
		prVals = map[string]float64{"N": 0.85, "L": 0.68, "H": 0.5}
	}
	pr, ok3 := look("PR", prVals)
	ui, ok4 := look("UI", map[string]float64{"N": 0.85, "R": 0.62})
	cia := map[string]float64{"H": 0.56, "L": 0.22, "N": 0}
	c, ok5 := look("C", cia)
	i, ok6 := look("I", cia)
	a, ok7 := look("A", cia)
	if !(ok1 && ok2 && ok3 && ok4 && ok5 && ok6 && ok7) {
		return 0, false
	}
	iss := 1 - (1-c)*(1-i)*(1-a)
	var impact float64
	if scope == "U" {
		impact = 6.42 * iss
	} else {
		impact = 7.52*(iss-0.029) - 3.25*math.Pow(iss-0.02, 15)
	}
	expl := 8.22 * av * ac * pr * ui
	if impact <= 0 {
		return 0, true
	}
	if scope == "U" {
		return roundUp(math.Min(impact+expl, 10)), true
	}
	return roundUp(math.Min(1.08*(impact+expl), 10)), true
}

// roundUp rounds up to one decimal as CVSS 3.1 prescribes (free of
// floating-point surprises).
func roundUp(x float64) float64 {
	n := int64(math.Round(x * 100000))
	if n%10000 == 0 {
		return float64(n) / 100000
	}
	return float64(n/10000+1) / 10
}

// Severity levels: 0 unknown, 1 low, 2 medium, 3 high, 4 critical.
func scoreLevel(score float64) int {
	switch {
	case score >= 9:
		return 4
	case score >= 7:
		return 3
	case score >= 4:
		return 2
	default:
		return 1
	}
}

// wordLevel maps the words distributions and advisories use.
func wordLevel(w string) int {
	switch strings.ToLower(strings.TrimSpace(w)) {
	case "critical":
		return 4
	case "high", "important":
		return 3
	case "medium", "moderate":
		return 2
	case "low", "negligible", "unimportant":
		return 1
	}
	return 0
}

// minorWord: what the distribution itself considers not worth fixing.
func minorWord(w string) bool {
	switch strings.ToLower(strings.TrimSpace(w)) {
	case "unimportant", "negligible", "end-of-life":
		return true
	}
	return false
}
