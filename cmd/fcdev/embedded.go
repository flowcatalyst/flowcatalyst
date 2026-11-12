package main

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

// embeddedPGVersion pins the PostgreSQL major the embedded dev database runs.
//
// We pin it explicitly rather than inheriting embedded-postgres' DefaultConfig
// default so that bumping the library can never silently change the major and
// break existing data directories. The major changes only when this constant
// does — and `fcdev db upgrade` handles the on-disk transition, because a
// Postgres major upgrade is NOT in-place: a cluster initialised by an older
// major refuses to start under a newer server binary.
const embeddedPGVersion = embeddedpostgres.V18

// newEmbeddedPG builds the embedded Postgres handle shared by `start`, `fresh`
// and `db upgrade`. The cluster lives in <dataPath>/data; binaries + runtime
// are cached (per version) under embeddedPGCacheDir(). The version is pinned to
// embeddedPGVersion. The caller owns Start()/Stop().
func newEmbeddedPG(dataPath string, port int, password string) (*embeddedpostgres.EmbeddedPostgres, error) {
	if err := validateEmbeddedDBPassword(password); err != nil {
		return nil, err
	}
	cacheDir := embeddedPGCacheDir()
	if err := os.MkdirAll(dataPath, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}
	if err := prepareEmbeddedPassword(dataPath, password); err != nil {
		return nil, err
	}
	return embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Version(embeddedPGVersion).
		Port(uint32(port)).
		DataPath(filepath.Join(dataPath, "data")).
		RuntimePath(filepath.Join(cacheDir, "runtime")).
		BinariesPath(filepath.Join(cacheDir, "bin")).
		Username("postgres").
		Password(password).
		Database("flowcatalyst").
		StartParameters(embeddedPGStartParameters).
		StartTimeout(60 * time.Second)), nil
}

// embeddedPGStartParameters are passed to the embedded server with -c. The
// library does not set a listen address itself, so the server would take
// listen_addresses from the postgresql.conf initdb writes (whose built-in
// default is already "localhost"); setting it here makes loopback-only
// explicit and independent of a hand-edited conf in an existing data
// directory. The embedded cluster is a developer database with a known
// superuser, so it must never be reachable from the network.
var embeddedPGStartParameters = map[string]string{
	"listen_addresses": "localhost",
}

// Embedded database credentials. The default stays postgres/postgres: a
// developer's own application connects to this database with that well-known
// URL, so changing it would break every existing setup. The password can be
// set with --embedded-db-password / FC_EMBEDDED_DB_PASSWORD for a developer
// who does not want the well-known value (a shared machine, say).
const (
	embeddedDBUser            = "postgres"
	defaultEmbeddedDBPassword = "postgres"
	embeddedDBPasswordEnv     = "FC_EMBEDDED_DB_PASSWORD"

	// embeddedPasswordMarker records, beside the data directory, a salted
	// PBKDF2 of the password the cluster was created with, so a later start
	// with a different password fails at once with a clear message instead of
	// spinning on authentication failures until the start timeout. A cluster
	// with no marker was created with the default password.
	embeddedPasswordMarker = "db-password.check"
)

// addEmbeddedDBPasswordFlag registers --embedded-db-password. The default is
// empty on purpose: the env value is resolved at run time so it never appears
// in --help output.
func addEmbeddedDBPasswordFlag(cmd *cobra.Command) {
	cmd.Flags().String("embedded-db-password", "", "password for the embedded Postgres superuser, set when the cluster is first created (env FC_EMBEDDED_DB_PASSWORD; default: the well-known development password)")
}

// resolveEmbeddedDBPassword: the flag wins, then the environment, then the
// default.
func resolveEmbeddedDBPassword(cmd *cobra.Command) string {
	if v, _ := cmd.Flags().GetString("embedded-db-password"); v != "" {
		return v
	}
	if v := os.Getenv(embeddedDBPasswordEnv); v != "" {
		return v
	}
	return defaultEmbeddedDBPassword
}

// validateEmbeddedDBPassword rejects values the embedded-postgres library
// cannot pass safely: it interpolates the password into a libpq key/value
// string unquoted.
func validateEmbeddedDBPassword(pw string) error {
	if pw == "" {
		return errors.New("embedded database password must not be empty")
	}
	for _, r := range pw {
		if r <= ' ' || r >= 0x7f || r == '\'' || r == '"' || r == '\\' {
			return errors.New("embedded database password must be printable ASCII without spaces, quotes or backslashes")
		}
	}
	return nil
}

// embeddedDBURL is the URL every fcdev command uses to reach the embedded
// database. With the default password it is the well-known
// postgresql://postgres:postgres@localhost:<port>/flowcatalyst URL.
func embeddedDBURL(password string, port int) string {
	u := url.URL{
		Scheme:   "postgresql",
		User:     url.UserPassword(embeddedDBUser, password),
		Host:     fmt.Sprintf("localhost:%d", port),
		Path:     "/flowcatalyst",
		RawQuery: "sslmode=disable",
	}
	return u.String()
}

func embeddedPasswordHash(password string, salt []byte) (string, error) {
	k, err := pbkdf2.Key(sha256.New, password, salt, 100_000, 32)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(salt) + "$" + hex.EncodeToString(k), nil
}

// prepareEmbeddedPassword is called before the cluster starts. On a new data
// directory it records the password the cluster is about to be created with;
// on an existing one it fails if the configured password differs from the
// one the cluster was created with (it never tries to change it).
func prepareEmbeddedPassword(dataPath, password string) error {
	marker := filepath.Join(dataPath, embeddedPasswordMarker)
	major, err := embeddedDataMajor(dataPath)
	if err != nil {
		return fmt.Errorf("read embedded PG_VERSION: %w", err)
	}
	if major == "" { // no cluster yet: it will be created with this password
		if password == defaultEmbeddedDBPassword {
			if err := os.Remove(marker); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("clear password marker: %w", err)
			}
			return nil
		}
		salt := make([]byte, 16)
		if _, err := rand.Read(salt); err != nil {
			return err
		}
		h, err := embeddedPasswordHash(password, salt)
		if err != nil {
			return err
		}
		if err := os.WriteFile(marker, []byte(h+"\n"), 0o600); err != nil {
			return fmt.Errorf("write password marker: %w", err)
		}
		return nil
	}

	mismatch := fmt.Errorf("the embedded Postgres data directory %s was created with a different password than the one configured "+
		"(--embedded-db-password / %s, or the default when neither is set); fcdev does not change an existing cluster's password. "+
		"Use the password it was created with, or wipe it with 'fcdev start --embedded-db-reset'", dataPath, embeddedDBPasswordEnv)
	b, err := os.ReadFile(marker)
	if errors.Is(err, os.ErrNotExist) { // created with the default password
		if password != defaultEmbeddedDBPassword {
			return mismatch
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("read password marker: %w", err)
	}
	saltHex, _, ok := strings.Cut(strings.TrimSpace(string(b)), "$")
	salt, derr := hex.DecodeString(saltHex)
	if !ok || derr != nil {
		return fmt.Errorf("password marker %s is unreadable; remove the data directory with --embedded-db-reset", marker)
	}
	want, err := embeddedPasswordHash(password, salt)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(want), []byte(strings.TrimSpace(string(b)))) != 1 {
		return mismatch
	}
	return nil
}

// pinnedPGMajor is the major number of embeddedPGVersion (e.g. "18" from
// "18.3.0").
func pinnedPGMajor() string { return majorOf(string(embeddedPGVersion)) }

func majorOf(v string) string {
	if before, _, ok := strings.Cut(v, "."); ok {
		return before
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
		"embedded Postgres data dir is PG%s but this fcdev embeds PG%s; a major "+
			"upgrade is not in-place. Run 'fcdev db upgrade' (backs up the old cluster, "+
			"re-initialises PG%s, re-runs migrations + seed) or "+
			"'fcdev start --embedded-db-reset' to wipe it",
		have, pinnedPGMajor(), pinnedPGMajor())
}
