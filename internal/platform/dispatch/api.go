package dispatch

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
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
func (s *State) routerConfig(ctx context.Context, _ *struct{}) (*routerConfigOutput, error) {
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
	return &routerConfigOutput{Body: doc}, nil
}

type routerConfigOutput struct {
	Body common.RouterConfig
}

// RouterConfigForTest exposes the handler's gate + body to tests in other
// packages. The handler itself stays unexported, like every other route's.
func (s *State) RouterConfigForTest(ctx context.Context) (common.RouterConfig, error) {
	out, err := s.routerConfig(ctx, nil)
	if err != nil {
		return common.RouterConfig{}, err
	}
	return out.Body, nil
}
