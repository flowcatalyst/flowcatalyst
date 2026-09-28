//go:build integration

package control

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	fncontrol "github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// TestAuth_NoPrincipalIsUnauthenticated pins that every control-plane
// handler rejects a request with no AuthContext attached — the platform-wide
// convention (auth.RequirePermission/RequireAnchor both answer
// Authorization("UNAUTHENTICATED", …)); the outer Authenticator middleware
// is what turns an actually-malformed bearer into a real 401 — see
// docs/function-runner-plan.md WP5's auth-model report note.
func TestAuth_NoPrincipalIsUnauthenticated(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	ctx := context.Background() // no auth.WithContext

	_, err := s.desired(ctx, &desiredInput{Pool: "default"})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "UNAUTHENTICATED")

	_, err = s.heartbeat(ctx, &heartbeatInput{Body: fncontrol.Heartbeat{RunnerID: "r", Pool: "default"}})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "UNAUTHENTICATED")

	_, err = s.artifact(ctx, &artifactInput{Digest: "0000000000000000000000000000000000000000000000000000000000000000"[:64]})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "UNAUTHENTICATED")

	_, err = s.emit(ctx, &apicommon.In[fncontrol.EmitRequest]{Body: fncontrol.EmitRequest{FunctionID: "fn_x", Version: 1}})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "UNAUTHENTICATED")
}

// TestAuth_MissingPermissionIsForbidden pins that an anchor principal
// without platform:function:runner:control is rejected (PERMISSION_REQUIRED,
// 403), and that anchor scope ALONE (a permission-holding but non-anchor
// principal) is also rejected (ANCHOR_REQUIRED) — see auth.
// CanControlFunctionRunner's doc comment for why both are required.
func TestAuth_MissingPermissionIsForbidden(t *testing.T) {
	t.Parallel()
	s := newTestState(t)

	noPerm := auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "sa_no_perm", Scope: auth.ScopeAnchor, Permissions: []string{"platform:function:function:view"},
	})
	_, err := s.desired(noPerm, &desiredInput{Pool: "default"})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")

	notAnchor := auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "sa_not_anchor", Scope: auth.ScopeClient, Clients: []string{"clt_x"},
		Permissions: []string{"platform:function:runner:control"},
	})
	_, err = s.desired(notAnchor, &desiredInput{Pool: "default"})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "ANCHOR_REQUIRED")

	// The happy path really does need both.
	ok := anchorCtx("platform:function:runner:control")
	out, err := s.desired(ok, &desiredInput{Pool: "default"})
	assert.NoError(t, err)
	assert.NotNil(t, out)
}
