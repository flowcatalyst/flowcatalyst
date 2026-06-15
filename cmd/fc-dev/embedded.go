package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

// embeddedPGVersion pins the PostgreSQL major the embedded dev database runs.
//
// We pin it explicitly rather than inheriting embedded-postgres' DefaultConfig
// default so that bumping the library can never silently change the major and
// break existing data directories. The major changes only when this constant
// does — and `fc-dev db upgrade` handles the on-disk transition, because a
// Postgres major upgrade is NOT in-place: a cluster initialised by an older
// major refuses to start under a newer server binary.
const embeddedPGVersion = embeddedpostgres.V18

// newEmbeddedPG builds the embedded Postgres handle shared by `start`, `fresh`
// and `db upgrade`. The cluster lives in <dataPath>/data; binaries + runtime
// are cached (per version) under embeddedPGCacheDir(). The version is pinned to
// embeddedPGVersion. The caller owns Start()/Stop().
func newEmbeddedPG(dataPath string, port int) (*embeddedpostgres.EmbeddedPostgres, error) {
	cacheDir := embeddedPGCacheDir()
	if err := os.MkdirAll(dataPath, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}
	return embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Version(embeddedPGVersion).
		Port(uint32(port)).
		DataPath(filepath.Join(dataPath, "data")).
		RuntimePath(filepath.Join(cacheDir, "runtime")).
		BinariesPath(filepath.Join(cacheDir, "bin")).
		Username("postgres").
		Password("postgres").
		Database("flowcatalyst").
		StartTimeout(60 * time.Second)), nil
}

// pinnedPGMajor is the major number of embeddedPGVersion (e.g. "18" from
// "18.3.0").
func pinnedPGMajor() string { return majorOf(string(embeddedPGVersion)) }

func majorOf(v string) string {
	if i := strings.IndexByte(v, '.'); i >= 0 {
		return v[:i]
	}
	return v
}

// embeddedDataMajor returns the major version of an existing embedded cluster,
// read from <dataPath>/data/PG_VERSION (which holds just the major for PG 10+).
// Returns "" when no cluster has been initialised yet.
func embeddedDataMajor(dataPath string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dataPath, "data", "PG_VERSION"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// assertEmbeddedVersionCompatible turns the cryptic "database files are
// incompatible with server" failure into an actionable message when the
// on-disk cluster's major differs from the pinned major. No-op when there is no
// cluster yet or the majors already match.
func assertEmbeddedVersionCompatible(dataPath string) error {
	have, err := embeddedDataMajor(dataPath)
	if err != nil {
		return fmt.Errorf("read embedded PG_VERSION: %w", err)
	}
	if have == "" || have == pinnedPGMajor() {
		return nil
	}
	return fmt.Errorf(
		"embedded Postgres data dir is PG%s but this fc-dev embeds PG%s; a major "+
			"upgrade is not in-place. Run 'fc-dev db upgrade' (backs up the old cluster, "+
			"re-initialises PG%s, re-runs migrations + seed) or "+
			"'fc-dev start --embedded-db-reset' to wipe it",
		have, pinnedPGMajor(), pinnedPGMajor())
}
