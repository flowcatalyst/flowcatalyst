package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

func TestSplitCSV(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", []string{}},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{" a , ,b ", []string{"a", "b"}}, // trims and drops empties
	}
	for _, c := range cases {
		assert.Equal(t, c.want, splitCSV(c.in), "splitCSV(%q)", c.in)
	}
}

func TestToListFilterNormalization(t *testing.T) {
	ac := &auth.AuthContext{
		PrincipalID: "prn_caller",
		Scope:       auth.ScopeClient,
		Clients:     []string{"cli_a", "cli_b"},
	}
	f := toListFilter(ac, &listInput{
		Type:      " user ",
		ClientID:  " cli_a ",
		Active:    "true",
		Q:         " Alice_100% ",
		Roles:     " app:admin , , app:viewer ",
		Page:      2,
		PageSize:  25,
		SortField: "name",
		SortOrder: "DESC",
	})
	assert.False(t, f.Visibility.Anchor)
	assert.Equal(t, []string{"cli_a", "cli_b"}, f.Visibility.AccessibleClients)
	assert.Equal(t, "USER", f.Type)
	assert.Equal(t, "cli_a", f.ClientID)
	require.NotNil(t, f.Active)
	assert.True(t, *f.Active)
	assert.Equal(t, "alice_100%", f.Query)
	assert.Equal(t, []string{"app:admin", "app:viewer"}, f.Roles)
	assert.Equal(t, "name", f.SortField)
	assert.True(t, f.SortDesc)
	assert.Equal(t, 2, f.Page)
	assert.Equal(t, 25, f.PageSize)

	// Unrecognized active value leaves Active nil (both active and inactive).
	fBoth := toListFilter(&auth.AuthContext{Scope: auth.ScopeAnchor}, &listInput{Active: "all"})
	assert.True(t, fBoth.Visibility.Anchor)
	assert.Nil(t, fBoth.Active)
	assert.False(t, fBoth.SortDesc)
}

func TestOrderByClauseWhitelist(t *testing.T) {
	assert.Contains(t, principal.OrderByClause("name", false), `LOWER(p.name) COLLATE "C" ASC`)
	assert.Contains(t, principal.OrderByClause("name", true), `LOWER(p.name) COLLATE "C" DESC`)
	assert.Contains(t, principal.OrderByClause("email", false), `COLLATE "C" ASC`)
	assert.Contains(t, principal.OrderByClause("createdAt", true), `p.created_at DESC`)
	// Unknown sort keys safely fall back to created_at rather than interpolating input.
	assert.Equal(t, `p.created_at ASC, p.id ASC`, principal.OrderByClause("'; DROP TABLE iam_principals;--", false))
}
