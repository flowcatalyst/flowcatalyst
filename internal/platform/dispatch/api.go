package dispatch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// State backs the router-config endpoint.
type State struct {
	Documents *DocumentBuilder
}

const tag = "dispatch"

// Register mounts GET /api/dispatch/router-config on the platform API.
//
// It is an ordinary authenticated API route, not a special case: the router
// presents a bearer token like any other caller. The alternative considered
// and rejected was serving it unauthenticated on the internal listener behind
// a network alias — that made every deployment reason about the exposure of
// its metrics port, while a credential means the same thing everywhere and
// fails loudly when it is missing.
func Register(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	apiroute.Get(g, "getRouterConfig", "/api/dispatch/router-config",
		"The router's {processingPools, queues} configuration document", s.routerConfig)
}

// routerConfig serves the document.
//
// Anchor-only, and gated on the dispatch-pool view permission: the document
// lists every client's queue names and URLs alongside the pool codes, so a
// client-scoped principal must never see it. The built-in platform:router role
// carries exactly that one permission, which is what a deployed router's
// service account is given.
func (s *State) routerConfig(ctx context.Context, in *routerConfigInput) (*routerConfigOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	if err := auth.CanReadDispatchPools(ac); err != nil {
		return nil, err
	}
	doc, err := s.Documents.Build(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "build router config failed", err)
	}
	// The ETag is a hash of the rendered document, so a router that polls
	// every few seconds (F29) costs a 304 while nothing changed. The document
	// is built either way; what the validator saves is the transfer and the
	// router's parse and compare.
	etag, err := etagOf(doc)
	if err != nil {
		return nil, usecase.Internal("ROUTER_CONFIG_ETAG", "rendering the router config failed", err)
	}
	if in != nil && etagMatches(in.IfNoneMatch, etag) {
		return &routerConfigOutput{Status: http.StatusNotModified, ETag: etag}, nil
	}
	return &routerConfigOutput{Status: http.StatusOK, ETag: etag, Body: &doc}, nil
}

type routerConfigInput struct {
	IfNoneMatch string `header:"If-None-Match" doc:"An ETag from an earlier answer; 304 when the document is unchanged"`
}

type routerConfigOutput struct {
	Status int
	ETag   string `header:"ETag"`
	Body   *common.RouterConfig
}

// etagOf is a strong validator for doc: a hash of its JSON, which is
// deterministic (the builder orders pools by code and tenants stably).
func etagOf(doc common.RouterConfig) (string, error) {
	b, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return `"` + hex.EncodeToString(sum[:16]) + `"`, nil
}

// etagMatches reports whether an If-None-Match header names etag (a list, with
// weak validators, or "*").
func etagMatches(header, etag string) bool {
	for v := range strings.SplitSeq(header, ",") {
		v = strings.TrimPrefix(strings.TrimSpace(v), "W/")
		if v == "*" || v == etag {
			return true
		}
	}
	return false
}

// RouterConfigForTest exposes the handler's gate + body to tests in other
// packages. The handler itself stays unexported, like every other route's.
func (s *State) RouterConfigForTest(ctx context.Context) (common.RouterConfig, error) {
	out, err := s.routerConfig(ctx, nil)
	if err != nil {
		return common.RouterConfig{}, err
	}
	return *out.Body, nil
}

// RouterConfigConditionalForTest runs the handler with an If-None-Match header
// and returns the status, the ETag and the document (nil on 304).
func (s *State) RouterConfigConditionalForTest(ctx context.Context, ifNoneMatch string) (int, string, *common.RouterConfig, error) {
	out, err := s.routerConfig(ctx, &routerConfigInput{IfNoneMatch: ifNoneMatch})
	if err != nil {
		return 0, "", nil, err
	}
	return out.Status, out.ETag, out.Body, nil
}
