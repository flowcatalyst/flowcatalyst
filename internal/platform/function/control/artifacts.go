package control

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/artifact"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

type artifactInput struct {
	Digest string `path:"digest" doc:"Lowercase hex sha256 of the artifact (64 characters)"`
}

// artifact serves GET /control/functions/artifacts/{digest} (plan §8.3):
// a 302 to a presigned URL when the store supports one, else the bytes
// streamed straight from the store — the runner holds no storage
// credentials either way. Only digests some fng_versions row references are
// served; anything else is 404, so a runner can never pull an artifact
// nobody published.
//
// *huma.StreamResponse (rather than a typed Body) is what lets one handler
// answer with either a redirect or a streamed body: both need to set
// headers/status from inside the callback, which a static response schema
// can't express, and streaming avoids buffering a whole (up to 64 MiB,
// see function/api.maxArtifactBytes) artifact in memory per request.
func (s *State) artifact(ctx context.Context, in *artifactInput) (*huma.StreamResponse, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	if err := auth.CanControlFunctionRunner(ac); err != nil {
		return nil, err
	}
	if !artifact.ValidDigest(in.Digest) {
		return nil, usecase.Validation("INVALID_DIGEST", "digest must be a lowercase hex sha256 (64 characters)")
	}

	runtime, ok, err := s.Repo.VersionRuntimeForDigest(ctx, in.Digest)
	if err != nil {
		return nil, usecase.Internal("REPO", "version_runtime_for_digest failed", err)
	}
	if !ok {
		return nil, httperror.NotFound("Artifact", in.Digest)
	}
	contentType := "application/wasm"
	if runtime == function.RuntimeJS {
		contentType = "text/javascript"
	}

	url, presigned, err := s.Artifacts.PresignGet(ctx, in.Digest, s.presignTTL())
	if err != nil {
		return nil, usecase.Internal("ARTIFACT_STORE", "presign failed", err)
	}
	if presigned {
		return &huma.StreamResponse{Body: func(hctx huma.Context) {
			hctx.SetHeader("Cache-Control", "no-store")
			hctx.SetHeader("Location", url)
			hctx.SetStatus(http.StatusFound)
		}}, nil
	}

	rc, err := s.Artifacts.Open(ctx, in.Digest)
	if errors.Is(err, artifact.ErrNotFound) {
		// The version row named this digest but the store has since lost
		// it (e.g. a dev FileStore wiped between runs) — still a 404, not
		// a 500: the runner's retry policy for "nothing to download" and
		// "never existed" is the same.
		return nil, httperror.NotFound("Artifact", in.Digest)
	}
	if err != nil {
		return nil, usecase.Internal("ARTIFACT_STORE", "open failed", err)
	}
	return &huma.StreamResponse{Body: func(hctx huma.Context) {
		defer rc.Close()
		hctx.SetHeader("Cache-Control", "no-store")
		hctx.SetHeader("Content-Type", contentType)
		hctx.SetStatus(http.StatusOK)
		_, _ = io.Copy(hctx.BodyWriter(), rc)
	}}, nil
}
