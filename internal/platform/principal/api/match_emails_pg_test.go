//go:build integration

package api

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
)

func seedMatchUser(t *testing.T, ctx context.Context, id, scope string, clientID *string, name, email string, typ string) {
	t.Helper()
	pool := testpg.Pool(t)
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, scope, client_id, name, active, email)
		 VALUES ($1, $2, $3, $4, $5, TRUE, $6)`,
		id, typ, scope, clientID, name, email)
	require.NoError(t, err)
}

func matchEmailsEmails(r MatchEmailsResponse) []string {
	out := make([]string, 0, len(r.Matches))
	for _, m := range r.Matches {
		out = append(out, *m.Email)
	}
	return out
}

func requireValidationCode(t *testing.T, err error, code string) {
	t.Helper()
	require.Error(t, err)
	var ue *usecase.Error
	require.True(t, errors.As(err, &ue), "want a usecase error, got %T: %v", err, err)
	assert.Equal(t, code, ue.Code)
	assert.Equal(t, usecase.KindValidation, ue.Kind)
}

func TestMatchEmails_AnchorSeesAllInInputOrder(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	s := &State{Repo: principal.NewRepository(pool)}
	ca, cb := "cli_matchem0001", "cli_matchem0002"
	seedMatchUser(t, ctx, "prn_matchem0001", "CLIENT", &ca, "Match A", "matchem-a@example.com", "USER")
	seedMatchUser(t, ctx, "prn_matchem0002", "CLIENT", &cb, "Match B", "matchem-b@example.com", "USER")
	seedMatchUser(t, ctx, "prn_matchem0003", "ANCHOR", nil, "Match C", "matchem-c@example.com", "USER")
	seedMatchUser(t, ctx, "prn_matchem0004", "CLIENT", &ca, "Match SVC", "matchem-svc@example.com", "SERVICE")

	anchor := auth.WithContext(ctx, &auth.AuthContext{PrincipalID: "prn_matchemadmin", Scope: auth.ScopeAnchor, Permissions: []string{testPermUserView}})
	got, err := s.MatchPrincipalEmails(anchor, MatchEmailsRequest{
		Emails: []string{"MatchEm-C@example.com"},
		Text:   "Bee <matchem-b@example.com>; nobody@example.com\nmatchem-a@example.com, matchem-c@example.com, junk, matchem-svc@example.com",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"matchem-c@example.com", "matchem-b@example.com", "matchem-a@example.com"}, matchEmailsEmails(got))
	assert.Equal(t, []string{"nobody@example.com", "matchem-svc@example.com"}, got.Unmatched, "a SERVICE principal is never a match")
	assert.Equal(t, []string{"junk"}, got.Invalid)
}

func TestMatchEmails_ClientAdminCannotSeeOtherClient(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	s := &State{Repo: principal.NewRepository(pool)}
	ca, cb := "cli_matchem0011", "cli_matchem0012"
	seedMatchUser(t, ctx, "prn_matchem0011", "CLIENT", &ca, "Mine", "matchem-mine@example.com", "USER")
	seedMatchUser(t, ctx, "prn_matchem0012", "CLIENT", &cb, "Theirs", "matchem-theirs@example.com", "USER")
	seedMatchUser(t, ctx, "prn_matchem0013", "ANCHOR", nil, "Platform", "matchem-platform@example.com", "USER")

	caller := auth.WithContext(ctx, clientScopedCaller("prn_matchemcaller", ca, testPermUserView))
	got, err := s.MatchPrincipalEmails(caller, MatchEmailsRequest{
		Text: "matchem-mine@example.com matchem-theirs@example.com matchem-platform@example.com",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"matchem-mine@example.com"}, matchEmailsEmails(got))
	assert.Equal(t, []string{"matchem-theirs@example.com", "matchem-platform@example.com"}, got.Unmatched,
		"users the caller may not see are reported as not found")
}

func TestMatchEmails_RequiresReadPermission(t *testing.T) {
	ctx := context.Background()
	s := &State{Repo: principal.NewRepository(testpg.Pool(t))}
	caller := auth.WithContext(ctx, clientScopedCaller("prn_matchemnoperm", "cli_matchem0021"))
	_, err := s.MatchPrincipalEmails(caller, MatchEmailsRequest{Text: "a@example.com"})
	require.Error(t, err)
}

func TestMatchEmails_Limits(t *testing.T) {
	ctx := context.Background()
	s := &State{Repo: principal.NewRepository(testpg.Pool(t))}
	anchor := auth.WithContext(ctx, &auth.AuthContext{PrincipalID: "prn_matchemadmin", Scope: auth.ScopeAnchor, Permissions: []string{testPermUserView}})

	_, err := s.MatchPrincipalEmails(anchor, MatchEmailsRequest{Text: strings.Repeat("a", 200_001)})
	requireValidationCode(t, err, "TEXT_TOO_LONG")

	var b strings.Builder
	for i := range 1001 {
		b.WriteString("u")
		b.WriteString(strings.Repeat("x", i%7))
		b.WriteString(string(rune('a' + i%26)))
		b.WriteString(strings.Repeat("y", i/26))
		b.WriteString("@example.com ")
	}
	_, err = s.MatchPrincipalEmails(anchor, MatchEmailsRequest{Text: b.String()})
	requireValidationCode(t, err, "TOO_MANY_EMAILS")

	_, err = s.MatchPrincipalEmails(anchor, MatchEmailsRequest{Text: " , ;\n", Emails: []string{""}})
	requireValidationCode(t, err, "EMAILS_REQUIRED")

	// Only-invalid input is not "nothing parsed": it reports the invalid entries.
	got, err := s.MatchPrincipalEmails(anchor, MatchEmailsRequest{Text: "junk"})
	require.NoError(t, err)
	assert.Equal(t, []string{"junk"}, got.Invalid)
	assert.Empty(t, got.Matches)
}
