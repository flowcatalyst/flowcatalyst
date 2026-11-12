//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// The field admin routes authenticate before they resolve the client, and a
// refused caller gets the same answer for an unknown client as for one it
// cannot reach: neither answer says whether a client exists.
func TestClientGate_NoClientOracle(t *testing.T) {
	f := newFixture(t, hasher(t))
	const unknown = "no-such-client-gate-oracle"
	existing := f.identifier()

	// A client-tier caller reaching some other client only.
	narrow := testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_clientgate_narrow", Scope: auth.ScopeClient, Clients: []string{"clt_someotherclient"},
		Permissions: []string{"platform:*:*:*"}, PrincipalType: auth.PrincipalTypeUser,
	})

	type call func(ctx context.Context, clientRef string) error
	calls := map[string]call{
		"listWorkers": func(ctx context.Context, ref string) error {
			_, err := f.s.listWorkers(ctx, &listWorkersInput{ClientID: ref})
			return err
		},
		"ensureWorker": func(ctx context.Context, ref string) error {
			_, err := f.s.ensureWorker(ctx, &apicommon.In[EnsureFieldWorkerRequest]{Body: EnsureFieldWorkerRequest{
				ClientID: ref, WorkerCode: "W-GATE", Name: "Gate", AppCodes: []string{f.codeA},
			}})
			return err
		},
		"listDevices": func(ctx context.Context, ref string) error {
			_, err := f.s.listDevices(ctx, &listDevicesInput{ClientID: ref})
			return err
		},
		"createCode": func(ctx context.Context, ref string) error {
			_, err := f.s.createCode(ctx, &apicommon.In[CreateEnrolmentCodeRequest]{Body: CreateEnrolmentCodeRequest{
				ClientID: ref, Mode: "SHARED", AppCodes: []string{f.codeA},
			}})
			return err
		},
		"getPolicy": func(ctx context.Context, ref string) error {
			_, err := f.s.getPolicy(ctx, &policyInput{ClientID: ref})
			return err
		},
		"putPolicy": func(ctx context.Context, ref string) error {
			_, err := f.s.putPolicy(ctx, &putPolicyInput{ClientID: ref, Body: PutFieldPolicyRequest{}})
			return err
		},
	}

	for name, do := range calls {
		t.Run(name, func(t *testing.T) {
			// Unauthenticated: the same answer whatever the client value.
			errUnknown := do(context.Background(), unknown)
			errExisting := do(context.Background(), existing)
			requireErr(t, errUnknown, usecase.KindAuthorization, "UNAUTHENTICATED")
			requireErr(t, errExisting, usecase.KindAuthorization, "UNAUTHENTICATED")
			assert.Equal(t, errUnknown.Error(), errExisting.Error())

			// Authenticated without reach: unknown and unreachable look alike.
			errUnknown = do(narrow, unknown)
			errExisting = do(narrow, existing)
			require.Error(t, errUnknown)
			require.Error(t, errExisting)
			assert.Equal(t, errUnknown.Error(), errExisting.Error())
			requireErr(t, errUnknown, usecase.KindAuthorization, "SCOPE_FORBIDDEN")

			// With reach: unknown is 404, an existing client works.
			requireErr(t, do(f.actx, unknown), usecase.KindNotFound, "Client_NOT_FOUND")
			assert.NotEqual(t, usecase.KindNotFound, kindOf(do(f.actx, existing)))
		})
	}
}

func kindOf(err error) usecase.Kind {
	if ue := usecase.AsError(err); ue != nil {
		return ue.Kind
	}
	return ""
}
