package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

// maxArtifactBytes caps a module download.
const maxArtifactBytes = 64 << 20

var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// artifactCache keeps downloaded modules on disk by digest and re-verifies
// the digest on every read. Without a directory it keeps them in memory.
type artifactCache struct {
	dir string
	cp  ControlPlane
	mu  sync.Mutex
	mem map[string][]byte
}

func newArtifactCache(cacheDir string, cp ControlPlane) (*artifactCache, error) {
	c := &artifactCache{cp: cp, mem: map[string][]byte{}}
	if cacheDir != "" {
		c.dir = filepath.Join(cacheDir, "artifacts")
		if err := os.MkdirAll(c.dir, 0o700); err != nil {
			return nil, fmt.Errorf("runner: artifact cache %s: %w", c.dir, err)
		}
	}
	return c, nil
}

func verifyDigest(b []byte, digest string) bool {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]) == digest
}

func (c *artifactCache) get(ctx context.Context, digest string) ([]byte, error) {
	if !digestPattern.MatchString(digest) {
		return nil, fmt.Errorf("digest %q is not a sha256 hex digest", digest)
	}
	if b, ok := c.cached(digest); ok {
		return b, nil
	}
	rc, err := c.cp.Artifact(ctx, digest)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, maxArtifactBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxArtifactBytes {
		return nil, errors.New("artifact is over 64 MiB")
	}
	if !verifyDigest(b, digest) {
		return nil, errors.New("artifact does not match its digest")
	}
	c.store(digest, b)
	return b, nil
}

func (c *artifactCache) path(digest string) string {
	return filepath.Join(c.dir, digest[:2], digest+".wasm")
}

func (c *artifactCache) cached(digest string) ([]byte, bool) {
	if c.dir == "" {
		c.mu.Lock()
		defer c.mu.Unlock()
		b, ok := c.mem[digest]
		return b, ok
	}
	b, err := os.ReadFile(c.path(digest))
	if err != nil {
		return nil, false
	}
	if !verifyDigest(b, digest) {
		_ = os.Remove(c.path(digest)) // corrupt on disk: fetch again
		return nil, false
	}
	return b, true
}

func (c *artifactCache) store(digest string, b []byte) {
	if c.dir == "" {
		c.mu.Lock()
		c.mem[digest] = b
		c.mu.Unlock()
		return
	}
	p := c.path(digest)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".dl-*")
	if err != nil {
		return
	}
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(tmp.Name())
		return
	}
	_ = os.Rename(tmp.Name(), p)
}
