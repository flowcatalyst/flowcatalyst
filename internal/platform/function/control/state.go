// Package control is the platform's side of the function runner's control
// plane (docs/function-runner-plan.md §8.3): the desired-state document a
// runner long-polls, its heartbeat, artifact downloads, and event emission
// on a function's behalf. internal/functions/control (imported here as
// fncontrol) is the wire contract BOTH sides share — the runner's
// fncontrol.Client calls exactly the routes this package registers.
//
// Registered on the platform's ordinary huma API, under the platform's
// normal bearer-token auth middleware, gated on the
// platform:function:runner:control permission plus anchor scope — the same
// posture as GET /api/dispatch/router-config (internal/platform/dispatch,
// the router's analogous control-plane document): a credential means the
// same thing everywhere and fails loudly when it is missing, so these
// routes are an ordinary authenticated route, not a special-cased
// unauthenticated surface on a different listener. They therefore appear in
// api/openapi.lock.json like every other platform route.
package control

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"

	fncontrol "github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/artifact"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
)

// OutboundCredsResolver resolves the service account credentials the
// platform signs a function's deliveries with, by application id — pass
// serviceaccount.NewCachedOutboundCredsResolver(repo, ttl). WebhookSecret in
// the desired document is OutboundCreds.SigningSecret: the same secret
// scheduled-job firings and subscription deliveries sign with (see
// internal/server/delivery_creds.go and
// internal/platform/scheduledjob/scheduler/dispatcher.go), so a function's
// webhook endpoint verifies exactly what the platform signs, whichever
// delivery path it arrived by.
type OutboundCredsResolver func(ctx context.Context, applicationID string) (serviceaccount.OutboundCreds, error)

// defaultReRenderInterval is how often a held long-poll re-renders the
// desired document while waiting for a NOTIFY (docs/function-runner-plan.md
// §8.3's fallback path). Overridable per State for tests that want to
// observe the fallback without a 15s wait.
const defaultReRenderInterval = 15 * time.Second

// defaultArtifactPresignTTL is how long a presigned artifact URL is valid.
const defaultArtifactPresignTTL = 15 * time.Minute

// State bundles the control plane's dependencies.
type State struct {
	Repo      *function.Repository
	Events    *event.Repository
	Artifacts artifact.Store
	// Creds resolves a function's webhook signing secret. Required; a nil
	// Creds serves every function with an empty WebhookSecret (a runner then
	// answers `webhook` endpoints as unverifiable), so wiring always sets it.
	Creds OutboundCredsResolver
	// Auth is the platform's token issuer + audience, reported verbatim in
	// every Desired document (fncontrol.Desired.Auth) so a runner validates
	// `auth: platform` bearer tokens against the same JWKS this process
	// signs with.
	Auth fncontrol.TokenAuth
	// Listener wakes a held long-poll on NOTIFY fng_desired. Nil is valid
	// (tests, or a deployment that hasn't wired one) — desired then falls
	// back to ReRenderInterval-only polling for the wait duration.
	Listener *Listener

	// ReRenderInterval overrides defaultReRenderInterval (tests).
	ReRenderInterval time.Duration
	// ArtifactPresignTTL overrides defaultArtifactPresignTTL.
	ArtifactPresignTTL time.Duration
}

func (s *State) reRenderInterval() time.Duration {
	if s.ReRenderInterval > 0 {
		return s.ReRenderInterval
	}
	return defaultReRenderInterval
}

func (s *State) presignTTL() time.Duration {
	if s.ArtifactPresignTTL > 0 {
		return s.ArtifactPresignTTL
	}
	return defaultArtifactPresignTTL
}

const tag = "function-control"

// Register mounts the control-plane routes on api.
func Register(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	apiroute.Get(g, "getFunctionRunnerDesired", fncontrol.PathDesired,
		"A function pool's desired state (long-poll; function-runner role only)", s.desired)
	apiroute.Post(g, "postFunctionRunnerHeartbeat", fncontrol.PathHeartbeat,
		"A function runner's heartbeat report (function-runner role only)", 204, s.heartbeat)
	apiroute.Get(g, "getFunctionRunnerArtifact", fncontrol.PathArtifacts+"/{digest}",
		"Download a function artifact by digest (function-runner role only)", s.artifact)
	apiroute.Post(g, "postFunctionRunnerEvent", fncontrol.PathEvents,
		"Emit an event on a function's behalf (function-runner role only)", 201, s.emit)
}
