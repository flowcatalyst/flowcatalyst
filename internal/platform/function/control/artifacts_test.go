//go:build integration

package control

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

func TestArtifact_UnknownDigestIs404(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	ctx := anchorCtx()

	unknown := strings.Repeat("0", 64) // valid shape, no version references it
	_, err := s.artifact(ctx, &artifactInput{Digest: unknown})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Artifact_NOT_FOUND")
}

func TestArtifact_InvalidDigestIsValidationError(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	ctx := anchorCtx()

	_, err := s.artifact(ctx, &artifactInput{Digest: "not-a-digest"})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "INVALID_DIGEST")
}

// TestArtifact_StreamsFromFileStore pins the end-to-end download: a digest
// referenced by a version streams the stored bytes with a runtime-derived
// Content-Type, over a real HTTP round trip.
func TestArtifact_StreamsFromFileStore(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	appID := "app_" + shortID(t)
	appCode := "artapp" + shortID(t)
	seedApplication(t, testpg.Pool(t), appID, appCode)
	fn := seedFunction(t, testpg.Pool(t), s.Repo, appID, appCode, "fn", nil)

	body := []byte("\x00asm-fake-bytes-for-test")
	digest := sha256Hex(body)
	require.NoError(t, s.Artifacts.Put(context.Background(), digest, bytes.NewReader(body)))
	v := seedVersionWithDigest(t, s.Repo, fn.ID, 1, function.VersionPublished, function.RuntimeJS, describeJSON(t), digest)

	_, hapi := humatest.New(t)
	huma.Register(hapi, huma.Operation{
		OperationID: "getArtifact", Method: "GET", Path: "/control/functions/artifacts/{digest}",
	}, s.artifact)

	rec := hapi.GetCtx(anchorCtx(), "/control/functions/artifacts/"+v.Digest)

	require.Equal(t, 200, rec.Code, rec.Body.String())
	assert.Equal(t, "text/javascript", rec.Header().Get("Content-Type"))
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	assert.Equal(t, body, rec.Body.Bytes())
}
