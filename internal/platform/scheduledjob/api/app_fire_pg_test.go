//go:build integration

package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// An application's service account looks up (by id, by code) and fires jobs of
// its own application only; the messaging grants are unchanged.
func TestAppServiceFireAndLookup_ConfinedToOwnApplication(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	instances := scheduledjob.NewInstanceRepository(pool)
	s := &State{Repo: scheduledjob.NewRepository(pool), Instances: instances, UoW: testpg.NewUoW(t)}

	const ownApp, otherApp = "app_affown000001", "app_affother0001"
	mk := func(id, code string, applicationID *string) {
		t.Helper()
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_scheduled_jobs (id, code, name, status, crons, application_id)
			 VALUES ($1, $2, $2, 'ACTIVE', '{}', $3)`, id, code, applicationID)
		require.NoError(t, err)
	}
	own, other := ownApp, otherApp
	mk("sjb_affown000001", "aff-own-job", &own)
	mk("sjb_affother0001", "aff-other-job", &other)
	mk("sjb_affnoapp0001", "aff-noapp-job", nil)

	appSA := func(perm string) context.Context {
		return testpg.WithAuth(ctx, &auth.AuthContext{
			PrincipalID: "prn_affappsa0001", Scope: auth.ScopeAnchor, PrincipalType: "SERVICE",
			Applications: []string{ownApp}, Permissions: []string{perm},
		})
	}
	const fire = "platform:application-service:scheduled-job:fire"
	const sync = "platform:application-service:scheduled-job:sync"

	status := func(t *testing.T, err error) int {
		t.Helper()
		if err == nil {
			return http.StatusOK
		}
		ue := usecase.AsError(err)
		require.NotNil(t, ue, "unexpected error %v", err)
		if ue.Kind == usecase.KindAuthorization {
			return http.StatusForbidden
		}
		if ue.Kind == usecase.KindNotFound {
			return http.StatusNotFound
		}
		t.Fatalf("unexpected kind %v (%v)", ue.Kind, err)
		return 0
	}
	fireJob := func(c context.Context, id string) error {
		_, err := s.fireNow(c, &fireNowInput{ID: id})
		return err
	}
	byID := func(c context.Context, id string) error {
		_, err := s.getByID(c, &apicommon.IDInput{ID: id})
		return err
	}
	byCode := func(c context.Context, code string) error {
		_, err := s.getByCode(c, &byCodeInput{Code: code})
		return err
	}

	// Fire.
	assert.Equal(t, 200, status(t, fireJob(appSA(fire), "sjb_affown000001")))
	assert.Equal(t, 403, status(t, fireJob(appSA(fire), "sjb_affother0001")))
	assert.Equal(t, 403, status(t, fireJob(appSA(fire), "sjb_affnoapp0001")))
	assert.Equal(t, 403, status(t, fireJob(appSA(sync), "sjb_affown000001")), "sync does not fire")

	// Look-up by id and by code, with the fire or the sync grant.
	for _, p := range []string{fire, sync} {
		assert.Equal(t, 200, status(t, byID(appSA(p), "sjb_affown000001")))
		assert.Equal(t, 404, status(t, byID(appSA(p), "sjb_affother0001")))
		assert.Equal(t, 404, status(t, byID(appSA(p), "sjb_affnoapp0001")))
		assert.Equal(t, 200, status(t, byCode(appSA(p), "aff-own-job")))
		assert.Equal(t, 404, status(t, byCode(appSA(p), "aff-other-job")))
		assert.Equal(t, 404, status(t, byCode(appSA(p), "aff-noapp-job")))
	}

	// all_applications reaches every application's job, never a job of none.
	all := testpg.WithAuth(ctx, &auth.AuthContext{
		PrincipalID: "prn_affallapp001", Scope: auth.ScopeAnchor, AllApplications: true,
		Permissions: []string{fire},
	})
	assert.Equal(t, 200, status(t, fireJob(all, "sjb_affother0001")))
	assert.Equal(t, 403, status(t, fireJob(all, "sjb_affnoapp0001")))

	// Messaging grants unchanged: any job, including one of no application.
	admin := testpg.WithAuth(ctx, &auth.AuthContext{
		PrincipalID: "prn_affadmin0001", Scope: auth.ScopeAnchor,
		Permissions: []string{"platform:messaging:scheduled-job:fire", "platform:messaging:scheduled-job:view"},
	})
	assert.Equal(t, 200, status(t, fireJob(admin, "sjb_affnoapp0001")))
	assert.Equal(t, 200, status(t, byCode(admin, "aff-other-job")))

	// No relevant grant at all.
	none := testpg.WithAuth(ctx, &auth.AuthContext{
		PrincipalID: "prn_affnone00001", Scope: auth.ScopeAnchor, Applications: []string{ownApp},
		Permissions: []string{"platform:application-service:scheduled-job-instance:write"},
	})
	assert.Equal(t, 403, status(t, fireJob(none, "sjb_affown000001")))
	assert.Equal(t, 403, status(t, byCode(none, "aff-own-job")))
}
