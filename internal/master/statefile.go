package master

// The state file: gzip-compressed and scrambled (obfuscation, not encryption).

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
	maxStateFile = 512 << 20
)

var stateMagic = []byte("COREMON-STATE-1\n")

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

func readStateJSON(dir string) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(dir, stateFile))
	if err != nil {
		return nil, err
	}
	return decodeState(b)
}

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
