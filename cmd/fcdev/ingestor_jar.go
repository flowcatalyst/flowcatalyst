package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The ingestor runtime jar is found, in order:
//
//  1. FC_INGESTOR_JAR, an explicit path (an error if it does not exist);
//  2. a locally built jar, when fcdev is run from a checkout of the platform
//     repository (ingestor/ingestor-runtime/target/ingestor-runtime-*-all.jar,
//     the newest);
//  3. a cached download of the pinned release.
//
// No runtime release exists yet, so the release constants are empty; step 3
// is wired and tested but inert until they are filled in.
const (
	// ingestorReleaseVersion is the pinned runtime release, e.g. "0.1.0".
	ingestorReleaseVersion = ""
	// ingestorReleaseURLTemplate is where its jar is downloaded from; {version}
	// is replaced by the version.
	ingestorReleaseURLTemplate = ""
	// ingestorReleaseSHA256 is the jar's SHA-256, lower-case hex.
	ingestorReleaseSHA256 = ""
)

// ingestorRelease is the pinned release. A variable so tests can supply one.
var ingestorRelease = ingestorReleaseSpec{
	Version: ingestorReleaseVersion, URLTemplate: ingestorReleaseURLTemplate, SHA256: ingestorReleaseSHA256,
}

type ingestorReleaseSpec struct {
	Version, URLTemplate, SHA256 string
}

func (r ingestorReleaseSpec) configured() bool {
	return r.Version != "" && r.URLTemplate != "" && r.SHA256 != ""
}

func (r ingestorReleaseSpec) url() string {
	return strings.ReplaceAll(r.URLTemplate, "{version}", r.Version)
}

// jarSource says where a jar came from, for the logs and `fcdev ingestor status`.
type jarSource string

const (
	jarExplicit jarSource = "FC_INGESTOR_JAR"
	jarLocal    jarSource = "local build"
	jarRelease  jarSource = "released"
)

// ingestorJarCacheDir holds downloaded release jars.
func ingestorJarCacheDir() string {
	if d := os.Getenv("FC_INGESTOR_JRE_DIR"); d != "" {
		return filepath.Join(d, "ingestor-jars")
	}
	if c, err := os.UserCacheDir(); err == nil {
		return filepath.Join(c, "flowcatalyst", "fcdev-ingestor")
	}
	return filepath.Join(".", ".flowcatalyst-cache", "fcdev-ingestor")
}

// errNoIngestorJar means no jar could be found and none is released: not a
// failure, the ingestor is just not run.
var errNoIngestorJar = errors.New("no ingestor runtime jar found: build it with `ingestor/mvnw -f ingestor/pom.xml -DskipTests package` or set FC_INGESTOR_JAR; none is released yet")

// resolveIngestorJar finds the jar by the order above. startDirs are where the
// search for a checkout starts (the working directory and the executable's).
func resolveIngestorJar(ctx context.Context, explicit string, startDirs []string, rel ingestorReleaseSpec, cacheDir string, client *http.Client) (string, jarSource, error) {
	if explicit != "" {
		st, err := os.Stat(explicit) //nolint:gosec // G703: FC_INGESTOR_JAR is the developer's own setting
		if err != nil || st.IsDir() {
			return "", "", fmt.Errorf("FC_INGESTOR_JAR %q is not a readable file", explicit)
		}
		return explicit, jarExplicit, nil
	}
	if jar := localIngestorJar(startDirs); jar != "" {
		return jar, jarLocal, nil
	}
	if !rel.configured() {
		return "", "", errNoIngestorJar
	}
	jar := filepath.Join(cacheDir, rel.Version, "ingestor-runtime-"+rel.Version+"-all.jar")
	if st, err := os.Stat(jar); err == nil && !st.IsDir() {
		return jar, jarRelease, nil
	}
	// Download beside the destination and rename: an interrupted download
	// never leaves a jar that looks complete.
	part := jar + ".part"
	defer func() { _ = os.Remove(part) }()
	if err := downloadVerified(ctx, client, rel.url(), part, rel.SHA256); err != nil {
		return "", "", fmt.Errorf("download the ingestor runtime %s: %w", rel.Version, err)
	}
	if err := os.Rename(part, jar); err != nil {
		return "", "", err
	}
	slog.Info("downloaded the ingestor runtime", "version", rel.Version, "jar", jar)
	return jar, jarRelease, nil
}

// localIngestorJar looks for a built jar in a checkout at or above any of dirs
// and returns the newest, or "".
func localIngestorJar(dirs []string) string {
	for _, d := range dirs {
		for dir := d; dir != "" && dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
			if _, err := os.Stat(filepath.Join(dir, "ingestor", "pom.xml")); err != nil {
				continue
			}
			var best string
			var bestTime time.Time
			for _, pat := range []string{
				filepath.Join(dir, "ingestor", "ingestor-runtime", "target", "ingestor-runtime-*-all.jar"),
				filepath.Join(dir, "ingestor", "target", "ingestor-runtime-*-all.jar"),
			} {
				matches, _ := filepath.Glob(pat)
				for _, m := range matches {
					if st, err := os.Stat(m); err == nil && !st.IsDir() && (best == "" || st.ModTime().After(bestTime)) {
						best, bestTime = m, st.ModTime()
					}
				}
			}
			if best != "" {
				return best
			}
		}
	}
	return ""
}

// checkoutSearchDirs are the directories a checkout is looked for from.
func checkoutSearchDirs() []string {
	var dirs []string
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, wd)
	}
	if exe, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		dirs = append(dirs, filepath.Dir(exe))
	}
	return dirs
}
