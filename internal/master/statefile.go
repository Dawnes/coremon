package master

// The state file on disk: gzip-compressed and scrambled with a fixed byte
// sequence, so it is not human readable (and much smaller). This is
// obfuscation, not encryption: the scheme is in the (public) source code.
// `coremon-master show-state` prints it readably.

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

const (
	stateFile    = "state.dat"
	oldStateFile = "state.json" // before 0.3.12: plain JSON
	maxStateFile = 512 << 20
)

// stateMagic marks the format (and its version).
var stateMagic = []byte("COREMON-STATE-1\n")

// scramble XORs b in place with a fixed pseudo-random byte stream (SHA-256
// of a constant and a counter). Applying it twice restores the data.
func scramble(b []byte) {
	var block [sha256.Size]byte
	var ctr [8]byte
	for i := range b {
		if i%sha256.Size == 0 {
			binary.BigEndian.PutUint64(ctr[:], uint64(i/sha256.Size))
			block = sha256.Sum256(append([]byte("coremon state obfuscation v1"), ctr[:]...))
		}
		b[i] ^= block[i%sha256.Size]
	}
}

// encodeState turns the JSON state into the stored form.
func encodeState(js []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(js); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	z := buf.Bytes()
	scramble(z)
	return append(append([]byte{}, stateMagic...), z...), nil
}

// decodeState turns the stored form back into JSON.
func decodeState(b []byte) ([]byte, error) {
	if !bytes.HasPrefix(b, stateMagic) {
		return nil, errors.New("not a Coremon state file")
	}
	z := append([]byte{}, b[len(stateMagic):]...)
	scramble(z)
	zr, err := gzip.NewReader(bytes.NewReader(z))
	if err != nil {
		return nil, errors.New("state file is damaged")
	}
	js, err := io.ReadAll(io.LimitReader(zr, maxStateFile))
	if err != nil {
		return nil, errors.New("state file is damaged")
	}
	return js, nil
}

// readStateJSON returns the stored state as JSON: from state.dat, or from a
// plain state.json of an older version (converted on the next write).
// os.IsNotExist(err) when neither exists.
func readStateJSON(dir string) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(dir, stateFile))
	if err == nil {
		return decodeState(b)
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	return os.ReadFile(filepath.Join(dir, oldStateFile))
}

// ShowState returns the state readably (indented JSON), for
// `coremon-master show-state`.
func ShowState(dir string) ([]byte, error) {
	js, err := readStateJSON(dir)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := json.Indent(&out, js, "", "  "); err != nil {
		return nil, err
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}
