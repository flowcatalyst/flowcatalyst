package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
)

// This file is the CI half of `fcdev fn publish|deploy`: digest comparison
// (publish is idempotent per digest on the platform; the CLI reads first so a
// pipeline can tell "new" from "unchanged" and skip every write), --no-promote,
// --skip-unchanged and --json. docs/fcdev-profiles.md "Using fcdev in CI".

// fnDescribeArtifact is describeArtifact; a var so deploy tests need no engine.
var fnDescribeArtifact = describeArtifact

// digestHex is the sha256 of an artifact, lower-case hex: the platform's
// version identity.
func digestHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// short is the first 12 characters of a digest, as every message prints it.
func short(digest string) string {
	if len(digest) > 12 {
		return digest[:12]
	}
	return digest
}

// sameDigest compares two digests, tolerating case and a "sha256:" prefix.
func sameDigest(a, b string) bool {
	norm := func(s string) string { return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(s), "sha256:")) }
	return a != "" && norm(a) == norm(b)
}

// findDigest returns the function's version holding digest, or nil. Read-only.
func (a *fnAPI) findDigest(ctx context.Context, fn *fnFunction, digest string) (*fnVersion, error) {
	vs, err := a.listVersions(ctx, fn.ID)
	if err != nil {
		return nil, fmt.Errorf("list versions of %s: %w", fn.Address, err)
	}
	for i := range vs {
		if sameDigest(vs[i].Digest, digest) {
			return &vs[i], nil
		}
	}
	return nil, nil
}

// liveVersion returns the version the live alias points at, or 0 when there
// is none. Read-only.
func (a *fnAPI) liveVersion(ctx context.Context, fn *fnFunction) (int32, error) {
	var raw json.RawMessage
	if err := a.json(ctx, http.MethodGet, "/api/functions/"+fn.ID+"/aliases", nil, &raw); err != nil {
		return 0, fmt.Errorf("list aliases of %s: %w", fn.Address, err)
	}
	var aliases []fnAlias
	if json.Unmarshal(raw, &aliases) != nil {
		var page struct {
			Data []fnAlias `json:"data"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return 0, fmt.Errorf("list aliases of %s: %w", fn.Address, err)
		}
		aliases = page.Data
	}
	for _, al := range aliases {
		if al.Name == "live" {
			return al.Version, nil
		}
	}
	return 0, nil
}

// publishBytes uploads the artifact and publishes it as a version.
func (a *fnAPI) publishBytes(ctx context.Context, fn *fnFunction, data []byte, digest, runtime string) (*fnVersion, error) {
	ctype := "application/wasm"
	if runtime == "js" {
		ctype = "text/javascript"
	}
	if err := a.do(ctx, http.MethodPut, "/api/functions/"+fn.ID+"/artifacts/"+digest, bytes.NewReader(data), ctype, nil); err != nil {
		return nil, fmt.Errorf("upload: %w", err)
	}
	var v fnVersion
	if err := a.json(ctx, http.MethodPost, "/api/functions/"+fn.ID+"/versions", map[string]any{"digest": digest, "runtime": runtime}, &v); err != nil {
		return nil, fmt.Errorf("publish: %w", err)
	}
	return &v, nil
}

// fnReport routes a command's output. Human lines go to stdout, or to stderr
// under --json so stdout carries exactly one JSON object.
type fnReport struct {
	json   bool
	human  io.Writer
	stdout io.Writer
}

func newFnReport(cmd *cobra.Command, asJSON bool) *fnReport {
	r := &fnReport{json: asJSON, human: cmd.OutOrStdout(), stdout: cmd.OutOrStdout()}
	if asJSON {
		r.human = cmd.ErrOrStderr()
	}
	return r
}

func (r *fnReport) say(format string, args ...any) { fmt.Fprintf(r.human, format+"\n", args...) }

// emit writes the single JSON object under --json; otherwise nothing.
func (r *fnReport) emit(v any) error {
	if !r.json {
		return nil
	}
	enc := json.NewEncoder(r.stdout)
	return enc.Encode(v)
}

// fnPublishJSON is `publish --json`. created: a new version was created by
// this run (false: the digest already existed).
type fnPublishJSON struct {
	Address    string `json:"address"`
	FunctionID string `json:"functionId"`
	Version    int32  `json:"version"`
	Digest     string `json:"digest"`
	Created    bool   `json:"created"`
	Runtime    string `json:"runtime"`
	Client     string `json:"client,omitempty"`
}

// fnDeployJSON is `deploy --json`. dryRun appears only on a dry run.
type fnDeployJSON struct {
	Address    string `json:"address"`
	FunctionID string `json:"functionId,omitempty"`
	Version    int32  `json:"version"`
	Digest     string `json:"digest"`
	Created    bool   `json:"created"`
	Promoted   bool   `json:"promoted"`
	Skipped    bool   `json:"skipped"`
	Runtime    string `json:"runtime"`
	Client     string `json:"client,omitempty"`
	DryRun     bool   `json:"dryRun,omitempty"`
}

// ownerClient is the owning client id to report: the function's, else the one
// being set at creation.
func ownerClient(fn *fnFunction, owner fnClient) string {
	if fn != nil && fn.ClientID != nil && *fn.ClientID != "" {
		return *fn.ClientID
	}
	return owner.ID
}

// deployPlan is everything deploy decided before writing anything.
type deployPlan struct {
	fn            *fnFunction // nil: the function would be created
	address       string
	artifact      string
	runtime       string
	describe      *abi.Describe
	digest        string
	size          int
	wait          time.Duration
	owner         fnClient
	existing      *fnVersion // the version already holding the digest, or nil
	live          int32      // the live alias's version, 0 when none
	noPromote     bool
	skipUnchanged bool
}

// skip reports that nothing needs doing: live already has this digest.
func (p *deployPlan) skip() bool {
	return p.skipUnchanged && p.existing != nil && p.live == p.existing.Number
}

// needsWrite reports whether the run would write to the platform.
func (p *deployPlan) needsWrite() bool {
	return p.fn == nil || p.existing == nil || !p.noPromote
}

func (p *deployPlan) unchangedLive() string {
	return fmt.Sprintf("unchanged: %s live v%d already has digest %s", p.address, p.existing.Number, short(p.digest))
}

// dryRunReport prints what deploy would do. It performs no remote write.
func (p *deployPlan) dryRunReport(out io.Writer, platformURL string) {
	fmt.Fprintf(out, "dry run: nothing is written to %s\n", redactURL(platformURL))
	printDescribeTo(out, p.artifact, p.describe)
	fmt.Fprintf(out, "artifact: %s (%d bytes, sha256 %s, runtime %s)\n", p.artifact, p.size, short(p.digest), p.runtime)
	if p.fn == nil {
		fmt.Fprintf(out, "would create function %s\n", p.address)
		if p.owner.ID != "" {
			fmt.Fprintf(out, "owning client: %s\n", p.owner)
		}
	} else {
		fmt.Fprintf(out, "function %s exists (%s)\n", p.fn.Address, p.fn.ID)
	}
	switch {
	case p.skip():
		fmt.Fprintf(out, "%s\nwould do nothing (--skip-unchanged)\n", p.unchangedLive())
		return
	case p.existing != nil:
		fmt.Fprintf(out, "unchanged: %s v%d already has digest %s; would not upload\n", p.address, p.existing.Number, short(p.digest))
	default:
		fmt.Fprintf(out, "new: would upload the artifact and publish a new version of %s (%s, %s)\n", p.address, p.runtime, short(p.digest))
	}
	if p.existing == nil || p.existing.Status != "READY" {
		fmt.Fprintf(out, "would wait up to %s for a runner to load it\n", p.wait)
	}
	if p.noPromote {
		fmt.Fprintln(out, "would NOT promote it (--no-promote): live stays as it is")
	} else {
		fmt.Fprintln(out, "would promote it to live")
	}
}
