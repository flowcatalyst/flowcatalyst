package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
)

// ciPlatform is a fake platform holding one function (demo.logger, fn_1) with
// versions and a live alias, that records every write.
type ciPlatform struct {
	srv *httptest.Server
	mu  sync.Mutex

	exists   bool
	versions []fnVersion
	live     int32 // 0: no live alias
	created  bool
	writes   []string
	reads    []string
}

const ciAddress = "demo.logger"

func newCIPlatform(t *testing.T) *ciPlatform {
	p := &ciPlatform{exists: true}
	p.srv = httptest.NewServer(http.HandlerFunc(p.serve))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *ciPlatform) serve(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	key := r.Method + " " + r.URL.Path
	if r.URL.Path != "/oauth/token" {
		if r.Method == http.MethodGet {
			p.reads = append(p.reads, key)
		} else {
			p.writes = append(p.writes, key)
		}
	}
	const base = "/api/functions/fn_1"
	switch {
	case r.URL.Path == "/oauth/token":
		_, _ = fmt.Fprintf(w, `{"access_token":%q,"expires_in":300,"token_type":"Bearer"}`, jwt(`{"sub":"prn_T","tier":"ANCHOR"}`))
	case key == "GET /api/functions":
		if p.exists {
			_, _ = w.Write([]byte(`{"data":[{"id":"fn_1","address":"demo.logger","applicationCode":"demo"}]}`))
		} else {
			_, _ = w.Write([]byte(`{"data":[]}`))
		}
	case key == "GET /api/applications/by-code/demo":
		_, _ = w.Write([]byte(`{"id":"app_1"}`))
	case key == "POST /api/functions":
		p.exists, p.created = true, true
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	case key == "GET "+base+"/versions":
		_ = json.NewEncoder(w).Encode(p.versions)
	case strings.HasPrefix(key, "GET "+base+"/versions/"):
		var n int32
		_, _ = fmt.Sscan(strings.TrimPrefix(r.URL.Path, base+"/versions/"), &n)
		for _, v := range p.versions {
			if v.Number == n {
				_ = json.NewEncoder(w).Encode(v)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	case key == "GET "+base+"/aliases":
		if p.live == 0 {
			_, _ = w.Write([]byte(`[]`))
		} else {
			_, _ = fmt.Fprintf(w, `[{"name":"live","version":%d}]`, p.live)
		}
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, base+"/artifacts/"):
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusNoContent)
	case key == "POST "+base+"/versions":
		var body struct{ Digest, Runtime string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		for _, v := range p.versions { // idempotent per digest
			if v.Digest == body.Digest {
				_ = json.NewEncoder(w).Encode(v)
				return
			}
		}
		v := fnVersion{Number: int32(len(p.versions) + 1), Digest: body.Digest, Runtime: body.Runtime, Status: "READY"}
		p.versions = append(p.versions, v)
		_ = json.NewEncoder(w).Encode(v)
	case key == "PUT "+base+"/aliases/live":
		var body struct{ Version int32 }
		_ = json.NewDecoder(r.Body).Decode(&body)
		p.live = body.Version
		_, _ = w.Write([]byte(`{"wiring":{}}`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (p *ciPlatform) writeLog() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.writes...)
}

func (p *ciPlatform) liveNow() int32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.live
}

func (p *ciPlatform) versionCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.versions)
}

// ciEnv wires a profile (env np or prod) at the fake platform and writes an
// artifact. Non-terminal stdin, as in CI.
type ciEnv struct {
	p        *ciPlatform
	artifact string
	digest   string
}

func newCIEnv(t *testing.T, env string, profileExtra ...string) *ciEnv {
	t.Helper()
	e := newFnTestEnv(t)
	p := newCIPlatform(t)
	t.Setenv("FC_TEST_SECRET", testSecret)
	e.writeProfiles(t, 0o600, profileTOML("p", p.srv.URL, env, "env:FC_TEST_SECRET", profileExtra...))
	old := fnDescribeArtifact
	fnDescribeArtifact = func(context.Context, string, string) (*abi.Describe, []byte, error) {
		return &abi.Describe{}, nil, nil
	}
	t.Cleanup(func() { fnDescribeArtifact = old })
	art := filepath.Join(e.dir, "function.js")
	body := []byte(`(function(){ /* ci fixture */ })();`)
	require.NoError(t, os.WriteFile(art, body, 0o600))
	return &ciEnv{p: p, artifact: art, digest: digestHex(body)}
}

// withVersion seeds a version holding digest (READY).
func (c *ciEnv) withVersion(digest string) {
	c.p.versions = append(c.p.versions, fnVersion{Number: int32(len(c.p.versions) + 1), Digest: digest, Runtime: "js", Status: "READY"})
}

func (c *ciEnv) deploy(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	return runCobra(t, newFnDeployCmd(), "", append([]string{ciAddress, c.artifact, "--profile", "p"}, args...)...)
}

func (c *ciEnv) publish(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	return runCobra(t, newFnPublishCmd(), "", append([]string{ciAddress, c.artifact, "--profile", "p"}, args...)...)
}

// singleJSON decodes stdout, which must hold exactly one JSON object.
func singleJSON(t *testing.T, stdout string) map[string]any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(stdout))
	var m map[string]any
	require.NoError(t, dec.Decode(&m), "stdout must be one JSON object, got %q", stdout)
	require.False(t, dec.More(), "stdout has more than one JSON value: %q", stdout)
	require.Equal(t, "", strings.TrimSpace(stdout[dec.InputOffset():]))
	return m
}

// --- publish ----------------------------------------------------------------

func TestPublishNewVersionAndNeverPromotes(t *testing.T) {
	c := newCIEnv(t, "np")
	c.withVersion("0000000000000000000000000000000000000000000000000000000000000001")
	c.p.live = 1
	out, _, err := c.publish(t)
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("new: %s v2 (js, %s)\n", ciAddress, c.digest[:12]), out)
	require.EqualValues(t, 1, c.p.liveNow(), "publish must not move live")
	require.Contains(t, strings.Join(c.p.writeLog(), "\n"), "POST /api/functions/fn_1/versions")
	require.NotContains(t, strings.Join(c.p.writeLog(), "\n"), "aliases")
}

func TestPublishUnchangedDigestWritesNothing(t *testing.T) {
	c := newCIEnv(t, "np")
	c.withVersion("0000000000000000000000000000000000000000000000000000000000000001")
	c.withVersion(c.digest)
	out, _, err := c.publish(t)
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("unchanged: %s v2 already has digest %s\n", ciAddress, c.digest[:12]), out)
	require.Empty(t, c.p.writeLog(), "an unchanged digest must not upload or publish")
	require.Equal(t, 2, c.p.versionCount())
}

func TestPublishUnchangedNeedsNoProdConfirmation(t *testing.T) {
	c := newCIEnv(t, "prod") // non-terminal, no --yes, no allow_noninteractive
	c.withVersion(c.digest)
	_, _, err := c.publish(t)
	require.NoError(t, err)
	require.Empty(t, c.p.writeLog())
}

func TestPublishNewOnProdStillNeedsConfirmation(t *testing.T) {
	c := newCIEnv(t, "prod")
	_, _, err := c.publish(t)
	require.ErrorContains(t, err, "refusing to publish")
	require.Empty(t, c.p.writeLog())
	// --yes alone is not enough without allow_noninteractive ...
	_, _, err = c.publish(t, "--yes")
	require.ErrorContains(t, err, "allow_noninteractive")
	require.Empty(t, c.p.writeLog())
}

func TestPublishNewOnProdWithAllowNoninteractive(t *testing.T) {
	c := newCIEnv(t, "prod", "allow_noninteractive = true")
	_, _, err := c.publish(t, "--yes")
	require.NoError(t, err)
	require.NotEmpty(t, c.p.writeLog())
}

func TestPublishJSONIsOneObjectOnStdout(t *testing.T) {
	c := newCIEnv(t, "np")
	out, errOut, err := c.publish(t, "--json")
	require.NoError(t, err)
	m := singleJSON(t, out)
	require.Equal(t, map[string]any{
		"address": ciAddress, "functionId": "fn_1", "version": float64(1), "digest": c.digest,
		"created": true, "runtime": "js",
	}, m)
	require.Contains(t, errOut, "profile p (np) ", "the banner goes to stderr")
	require.Contains(t, errOut, "new: "+ciAddress+" v1")

	out, _, err = c.publish(t, "--json")
	require.NoError(t, err)
	m = singleJSON(t, out)
	require.Equal(t, false, m["created"])
	require.Equal(t, float64(1), m["version"])
	require.Equal(t, c.digest, m["digest"])
}

func TestPublishCreateJSONReportsClient(t *testing.T) {
	c := newCIEnv(t, "np")
	c.p.exists = false
	out, errOut, err := c.publish(t, "--create", "--client", testClientID, "--json")
	require.NoError(t, err)
	m := singleJSON(t, out)
	require.Equal(t, testClientID, m["client"])
	require.Equal(t, true, m["created"])
	require.Contains(t, errOut, "created "+ciAddress)
	require.NotContains(t, out, "created demo")
}

// --- deploy -----------------------------------------------------------------

func TestDeployNewVersionPromotes(t *testing.T) {
	c := newCIEnv(t, "np")
	out, _, err := c.deploy(t, "--json")
	require.NoError(t, err)
	m := singleJSON(t, out)
	require.Equal(t, map[string]any{
		"address": ciAddress, "functionId": "fn_1", "version": float64(1), "digest": c.digest,
		"created": true, "promoted": true, "skipped": false, "runtime": "js",
	}, m)
	require.EqualValues(t, 1, c.p.liveNow())
}

func TestDeployNoPromoteLeavesLiveUntouched(t *testing.T) {
	c := newCIEnv(t, "np")
	c.withVersion("0000000000000000000000000000000000000000000000000000000000000001")
	c.p.live = 1
	out, errOut, err := c.deploy(t, "--no-promote", "--json")
	require.NoError(t, err)
	m := singleJSON(t, out)
	require.Equal(t, true, m["created"])
	require.Equal(t, false, m["promoted"])
	require.Equal(t, float64(2), m["version"])
	require.EqualValues(t, 1, c.p.liveNow(), "--no-promote must not move live")
	for _, w := range c.p.writeLog() {
		require.NotContains(t, w, "/aliases/", "--no-promote must make no alias write")
	}
	require.Contains(t, errOut, "not promoted: "+ciAddress+" v2 is READY")
	require.Contains(t, strings.Join(c.p.reads, "\n"), "GET /api/functions/fn_1/versions/2", "it waits for READY")
}

func TestDeployNoPromoteHumanOutputMentionsNotPromoted(t *testing.T) {
	c := newCIEnv(t, "np")
	out, _, err := c.deploy(t, "--no-promote")
	require.NoError(t, err)
	require.Contains(t, out, "not promoted")
	require.NotContains(t, out, "@live →")
}

func TestDeploySkipUnchangedMakesNoWrite(t *testing.T) {
	c := newCIEnv(t, "prod") // no --yes, non-terminal: any write would be refused
	c.withVersion("0000000000000000000000000000000000000000000000000000000000000001")
	c.withVersion(c.digest)
	c.p.live = 2
	out, errOut, err := c.deploy(t, "--skip-unchanged")
	require.NoError(t, err, errOut)
	require.Equal(t, fmt.Sprintf("unchanged: %s live v2 already has digest %s\n", ciAddress, c.digest[:12]), out)
	require.Empty(t, c.p.writeLog())
}

func TestDeploySkipUnchangedJSON(t *testing.T) {
	c := newCIEnv(t, "np")
	c.withVersion(c.digest)
	c.p.live = 1
	out, errOut, err := c.deploy(t, "--skip-unchanged", "--json")
	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"address": ciAddress, "functionId": "fn_1", "version": float64(1), "digest": c.digest,
		"created": false, "promoted": false, "skipped": true, "runtime": "js",
	}, singleJSON(t, out))
	require.Contains(t, errOut, "unchanged: "+ciAddress+" live v1")
	require.Empty(t, c.p.writeLog())
}

func TestDeploySkipUnchangedDigestExistsButNotLive(t *testing.T) {
	c := newCIEnv(t, "np")
	c.withVersion(c.digest) // v1
	c.withVersion("0000000000000000000000000000000000000000000000000000000000000002")
	c.p.live = 2
	out, _, err := c.deploy(t, "--skip-unchanged", "--json")
	require.NoError(t, err)
	m := singleJSON(t, out)
	require.Equal(t, false, m["created"])
	require.Equal(t, true, m["promoted"])
	require.Equal(t, false, m["skipped"])
	require.Equal(t, float64(1), m["version"])
	require.EqualValues(t, 1, c.p.liveNow(), "the existing version is promoted")
	require.Equal(t, 2, c.p.versionCount(), "nothing is published")
	for _, w := range c.p.writeLog() {
		require.NotContains(t, w, "/artifacts/", "no upload for a known digest")
		require.NotContains(t, w, "POST /api/functions/fn_1/versions")
	}
}

func TestDeploySkipUnchangedNoLiveAliasPromotes(t *testing.T) {
	c := newCIEnv(t, "np")
	c.withVersion(c.digest)
	_, _, err := c.deploy(t, "--skip-unchanged")
	require.NoError(t, err)
	require.EqualValues(t, 1, c.p.liveNow())
}

func TestDeploySkipUnchangedWithNewDigestDeploys(t *testing.T) {
	c := newCIEnv(t, "np")
	c.withVersion("0000000000000000000000000000000000000000000000000000000000000001")
	c.p.live = 1
	_, _, err := c.deploy(t, "--skip-unchanged")
	require.NoError(t, err)
	require.Equal(t, 2, c.p.versionCount())
	require.EqualValues(t, 2, c.p.liveNow())
}

func TestDeployWithoutSkipUnchangedStillPromotesKnownDigest(t *testing.T) {
	c := newCIEnv(t, "np")
	c.withVersion(c.digest)
	c.p.live = 1
	out, _, err := c.deploy(t)
	require.NoError(t, err)
	require.Contains(t, out, "unchanged: "+ciAddress+" v1 already has digest")
	require.Contains(t, out, ciAddress+"@live → v1")
	require.Equal(t, 1, c.p.versionCount())
}

func TestDeployNoPromoteKnownDigestWritesNothingAndNeedsNoConfirmation(t *testing.T) {
	c := newCIEnv(t, "prod")
	c.withVersion("0000000000000000000000000000000000000000000000000000000000000001")
	c.withVersion(c.digest)
	c.p.live = 1
	_, _, err := c.deploy(t, "--no-promote")
	require.NoError(t, err)
	require.Empty(t, c.p.writeLog())
	require.EqualValues(t, 1, c.p.liveNow())
}

func TestDeployNoPromoteNewOnProdNeedsConfirmation(t *testing.T) {
	c := newCIEnv(t, "prod")
	_, _, err := c.deploy(t, "--no-promote")
	require.ErrorContains(t, err, "refusing to publish")
	require.Empty(t, c.p.writeLog())
	_, _, err = c.deploy(t, "--no-promote", "--yes")
	require.ErrorContains(t, err, "allow_noninteractive")
	require.Empty(t, c.p.writeLog())
}

func TestDeployProdPromoteNeedsConfirmationAndAllowNoninteractive(t *testing.T) {
	c := newCIEnv(t, "prod")
	_, _, err := c.deploy(t, "--skip-unchanged")
	require.ErrorContains(t, err, "refusing to deploy")
	require.Empty(t, c.p.writeLog())

	c2 := newCIEnv(t, "prod", "allow_noninteractive = true")
	_, _, err = c2.deploy(t, "--skip-unchanged", "--yes")
	require.NoError(t, err)
	require.EqualValues(t, 1, c2.p.liveNow())
}

func TestDeployCreatesMissingFunctionWithSkipUnchanged(t *testing.T) {
	c := newCIEnv(t, "np")
	c.p.exists = false
	out, _, err := c.deploy(t, "--skip-unchanged", "--create", "--client", testClientID, "--json")
	require.NoError(t, err)
	m := singleJSON(t, out)
	require.Equal(t, true, m["created"])
	require.Equal(t, true, m["promoted"])
	require.Equal(t, testClientID, m["client"])
	require.True(t, c.p.created)
}

func TestDeployMissingFunctionWithoutCreate(t *testing.T) {
	c := newCIEnv(t, "np")
	c.p.exists = false
	_, _, err := c.deploy(t)
	require.ErrorContains(t, err, "pass --create")
	require.Empty(t, c.p.writeLog())
}

func TestDeployJSONStdoutIsOnlyTheObject(t *testing.T) {
	c := newCIEnv(t, "np")
	out, errOut, err := c.deploy(t, "--json")
	require.NoError(t, err)
	singleJSON(t, out)
	require.Contains(t, errOut, "profile p (np) ")
	require.Contains(t, errOut, "new: ")
	require.Contains(t, errOut, "@live → v1")
}

// --- dry run ----------------------------------------------------------------

func TestDeployDryRunReportsNewUnchangedAndPromote(t *testing.T) {
	c := newCIEnv(t, "prod") // a dry run needs no confirmation
	out, _, err := c.deploy(t, "--dry-run")
	require.NoError(t, err)
	require.Contains(t, out, "new: would upload")
	require.Contains(t, out, "would promote it to live")

	out, _, err = c.deploy(t, "--dry-run", "--no-promote")
	require.NoError(t, err)
	require.Contains(t, out, "would NOT promote it (--no-promote)")

	c.withVersion(c.digest)
	out, _, err = c.deploy(t, "--dry-run")
	require.NoError(t, err)
	require.Contains(t, out, "unchanged: "+ciAddress+" v1 already has digest")
	require.Contains(t, out, "would promote it to live")

	c.p.live = 1
	out, _, err = c.deploy(t, "--dry-run", "--skip-unchanged")
	require.NoError(t, err)
	require.Contains(t, out, "unchanged: "+ciAddress+" live v1 already has digest "+c.digest[:12])
	require.Contains(t, out, "would do nothing")

	require.Empty(t, c.p.writeLog(), "a dry run writes nothing")
}

func TestDeployDryRunJSON(t *testing.T) {
	c := newCIEnv(t, "np")
	c.withVersion(c.digest)
	c.p.live = 1
	out, errOut, err := c.deploy(t, "--dry-run", "--skip-unchanged", "--json")
	require.NoError(t, err)
	m := singleJSON(t, out)
	require.Equal(t, true, m["dryRun"])
	require.Equal(t, true, m["skipped"])
	require.Equal(t, false, m["promoted"])
	require.Contains(t, errOut, "dry run")
	require.Empty(t, c.p.writeLog())
}

// --- helpers ----------------------------------------------------------------

func TestSameDigestAndShort(t *testing.T) {
	require.True(t, sameDigest("ABCdef", "abcDEF"))
	require.True(t, sameDigest("sha256:abc", "abc"))
	require.False(t, sameDigest("abc", "abd"))
	require.False(t, sameDigest("", ""))
	require.Equal(t, "0123456789ab", short("0123456789abcdef"))
	require.Equal(t, "abc", short("abc"))
}

// --- CI authentication ------------------------------------------------------

// TestCIProfileFromEnvironmentNonInteractive is the documented CI flow: no
// terminal, no keychain, no prompt; the profile is created from environment
// variables and a prod deploy then runs with --yes.
func TestCIProfileFromEnvironmentNonInteractive(t *testing.T) {
	e := newFnTestEnv(t) // tty=false
	p := newCIPlatform(t)
	oldRead := fnReadSecret
	fnReadSecret = func(string, io.Writer) (string, error) {
		t.Fatal("CI must never be prompted for a secret")
		return "", nil
	}
	t.Cleanup(func() { fnReadSecret = oldRead })
	oldDescribe := fnDescribeArtifact
	fnDescribeArtifact = func(context.Context, string, string) (*abi.Describe, []byte, error) {
		return &abi.Describe{}, nil, nil
	}
	t.Cleanup(func() { fnDescribeArtifact = oldDescribe })
	t.Setenv("FC_CI_PLATFORM", p.srv.URL)
	t.Setenv("FC_CI_CLIENT_ID", "oac_ci")
	t.Setenv("FC_CI_SECRET", testSecret)

	_, _, err := runCobra(t, newProfileCmd(), "", "add", "ci",
		"--platform", os.Getenv("FC_CI_PLATFORM"), "--env", "prod", "--allow-noninteractive",
		"--client-id", os.Getenv("FC_CI_CLIENT_ID"), "--secret-ref", "env:FC_CI_SECRET")
	require.NoError(t, err)
	require.Empty(t, e.kc.m, "no keychain entry may be written")
	body, err := os.ReadFile(e.profiles)
	require.NoError(t, err)
	require.NotContains(t, string(body), testSecret)

	art := filepath.Join(e.dir, "function.js")
	require.NoError(t, os.WriteFile(art, []byte(`(function(){})();`), 0o600))
	out, _, err := runCobra(t, newFnDeployCmd(), "", ciAddress, art, "--profile", "ci", "--skip-unchanged", "--yes", "--json")
	require.NoError(t, err)
	require.Equal(t, true, singleJSON(t, out)["promoted"])
}

func TestCIProfileAddWithoutSecretRefNeedsTerminal(t *testing.T) {
	newFnTestEnv(t) // tty=false
	_, _, err := runCobra(t, newProfileCmd(), "", "add", "ci", "--platform", "https://fc.example.com", "--env", "np", "--client-id", "x")
	require.ErrorContains(t, err, "--secret-ref env:NAME")
}
