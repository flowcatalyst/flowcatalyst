// Package pgbin starts the embedded PostgreSQL of the tests from one unpacked
// copy of its binaries per machine.
//
// The embedded-postgres library unpacks the PostgreSQL archive into the runtime
// directory on every start, and the tests give each test binary a runtime
// directory of its own. Unpacking is done in-process, so under the race detector
// it took about 30 seconds per integration package: most of an integration run.
// Here the binaries are unpacked once into a cache directory and every later
// start uses them; the data and runtime directories stay per test binary.
//
// The copy lives in <user cache dir>/flowcatalyst/testpg/postgres-<version>-<os>-<arch>,
// or in the directory FC_TESTPG_BINARIES names (a CI job can point it at a cached
// path). Deleting the directory is always safe: the next start unpacks it again.
//
// pkg/fcsdk/tests/internal/pgbin is the same file for the SDK's tests module,
// which cannot import this one; keep the two in step.
package pgbin

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

// Version is the PostgreSQL the tests run on. It is part of the cache
// directory's name, so changing it unpacks the new version beside the old.
const Version = embeddedpostgres.V18

// marker is written into the directory after a complete unpack: a directory
// without it is a partial unpack (the process died) and is unpacked again.
const marker = ".unpacked"

const (
	lockWait  = 3 * time.Minute  // longer than a cold unpack under the race detector
	lockStale = 10 * time.Minute // a lock this old was left by a dead process
)

// Dir returns the directory the binaries are kept in.
func Dir() (string, error) {
	if d := os.Getenv("FC_TESTPG_BINARIES"); d != "" {
		return d, nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "flowcatalyst", "testpg",
		fmt.Sprintf("postgres-%s-%s-%s", Version, runtime.GOOS, runtime.GOARCH)), nil
}

// Start starts an embedded PostgreSQL with cfg, on Version and from the shared
// binaries, unpacking them first when this machine has none. The caller sets
// the port, the data and runtime paths and the rest, and stops the instance.
// When the cache directory cannot be used the instance starts as it would
// without this package (its own unpack), so a test never fails for the cache.
func Start(cfg embeddedpostgres.Config) (*embeddedpostgres.EmbeddedPostgres, error) {
	cfg = cfg.Version(Version)
	dir, err := Dir()
	if err != nil {
		return start(cfg)
	}
	shared := cfg.BinariesPath(dir)
	if unpacked(dir) {
		return start(shared)
	}
	// Cold: one process unpacks, the others wait for it.
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return start(cfg)
	}
	unlock, err := lock(dir + ".lock")
	if err != nil {
		return start(cfg)
	}
	defer unlock()
	if unpacked(dir) {
		return start(shared)
	}
	if err := os.RemoveAll(dir); err != nil { // a partial unpack
		return start(cfg)
	}
	pg, err := start(shared)
	if err != nil {
		return nil, err
	}
	// The instance is up, so the unpack is complete. Without the marker the
	// next start unpacks again, which is slow and not wrong.
	_ = os.WriteFile(filepath.Join(dir, marker), []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o600)
	return pg, nil
}

func start(cfg embeddedpostgres.Config) (*embeddedpostgres.EmbeddedPostgres, error) {
	pg := embeddedpostgres.NewDatabase(cfg)
	if err := pg.Start(); err != nil {
		return nil, err
	}
	return pg, nil
}

func unpacked(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, marker))
	return err == nil
}

// lock takes the lock file at path, waiting for another process that holds it,
// and returns the function that releases it. A lock older than lockStale is
// taken over.
func lock(path string) (func(), error) {
	deadline := time.Now().Add(lockWait)
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
			_ = f.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if info, statErr := os.Stat(path); statErr == nil && time.Since(info.ModTime()) > lockStale {
			_ = os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("pgbin: %s is still held after %s", path, lockWait)
		}
		time.Sleep(250 * time.Millisecond)
	}
}
