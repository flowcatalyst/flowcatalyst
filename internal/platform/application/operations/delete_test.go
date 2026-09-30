package operations

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
)

// TestReferenceBlockers pins the APPLICATION_HAS_REFERENCES message parts to
// Rust's order and wording (fc-platform-iam application/operations/delete.rs).
func TestReferenceBlockers(t *testing.T) {
	t.Parallel()
	assert.Empty(t, referenceBlockers(application.References{}))
	assert.Equal(t, []string{
		"1 access grants", "2 client configs", "3 service accounts", "4 application roles", "5 principal refs",
	}, referenceBlockers(application.References{
		AccessGrants: 1, ClientConfigs: 2, ServiceAccounts: 3, Roles: 4, PrincipalRefs: 5,
	}))
	assert.Equal(t, []string{"7 application roles"}, referenceBlockers(application.References{Roles: 7}))
}
