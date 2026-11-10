package master

import (
	"os"
	"testing"
)

// The License page shows copies of the repository's LICENSE and
// THIRD-PARTY-NOTICES (embedded files cannot live outside the package);
// they must be identical.
func TestLegalTextsMatchRepository(t *testing.T) {
	for _, f := range [][2]string{
		{"../../LICENSE", "web/legal/LICENSE.txt"},
		{"../../THIRD-PARTY-NOTICES", "web/legal/THIRD-PARTY-NOTICES.txt"},
	} {
		want, err := os.ReadFile(f[0])
		if err != nil {
			t.Fatal(err)
		}
		got, err := webFS.ReadFile(f[1])
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Errorf("%s differs from %s: copy the repository file", f[1], f[0])
		}
	}
}
