package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"

	"coremon/internal/proto"
	"coremon/internal/sign"
)

func fetchVerified(ctx context.Context, cl *client, pub string, a *proto.Artifact, dst string) error {
	if a == nil {
		return errors.New("missing artifact")
	}
	key, err := sign.ParsePublic(pub)
	if err != nil {
		return err
	}
	if a.OS != runtime.GOOS || a.Arch != runtime.GOARCH {
		return fmt.Errorf("artifact is for %s/%s, this system is %s/%s", a.OS, a.Arch, runtime.GOOS, runtime.GOARCH)
	}

	if !sign.Verify(key, sign.Message(a.Kind, a.Name, a.Version, a.OS, a.Arch, a.SHA256), a.Signature) {
		return errors.New("signature verification FAILED - refusing artifact")
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
	if err != nil {
		return err
	}
	h := sha256.New()
	err = cl.download(ctx, a.URL, io.MultiWriter(f, h), 256<<20)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(dst)
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != a.SHA256 {
		os.Remove(dst)
		return fmt.Errorf("sha256 mismatch: got %s", got)
	}
	return nil
}
