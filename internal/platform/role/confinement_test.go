package role

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/go-sdk/usecase"
)

func TestConfinement(t *testing.T) {
	// The first segment names the application; a prefix match is not enough.
	assert.Empty(t, PermissionsOutsideApplication("orders", []string{"orders:order:view", "orders:*:*:*"}))
	assert.Equal(t, []string{"ordersx:a:b:c", "platform:*:*:*", "*:*:*:*", "bare"},
		PermissionsOutsideApplication("orders",
			[]string{"ordersx:a:b:c", "orders:a:b:c", "platform:*:*:*", "ordersx:a:b:c", "*:*:*:*", "bare"}))

	require.NoError(t, RequirePermissionsInApplication("orders", nil, false))
	require.NoError(t, RequirePermissionsInApplication("platform", []string{"platform:iam:user:view"}, false))
	err := RequirePermissionsInApplication("orders", []string{"orders:order:view", "billing:invoice:view", "platform:*:*:*"}, false)
	ue := usecase.AsError(err)
	require.NotNil(t, ue)
	assert.Equal(t, usecase.KindValidation, ue.Kind)
	assert.Equal(t, OutsideApplicationCode, ue.Code)
	assert.Equal(t, "Permission 'billing:invoice:view' does not belong to application 'orders'; "+
		"a role may only hold its own application's permissions", ue.Message)
	require.NoError(t, RequirePermissionsInApplication("orders", []string{"billing:invoice:view"}, true),
		"crossApplication lifts the rule")
}

func TestAddedPermissions(t *testing.T) {
	assert.Nil(t, AddedPermissions([]string{"a", "b"}, []string{"b", "a"}))
	assert.Nil(t, AddedPermissions([]string{"a", "b"}, nil), "removal adds nothing")
	assert.Equal(t, []string{"c", "d"}, AddedPermissions([]string{"a"}, []string{"a", "c", "d", "c"}))
}

func TestOwningApplicationCode(t *testing.T) {
	assert.Equal(t, "orders", New("orders", "clerk", "Clerk").OwningApplicationCode())
	assert.Equal(t, "legacy", Role{Name: "legacy:admin"}.OwningApplicationCode(), "a row without an application code")
	assert.Equal(t, "bare", Role{Name: "bare"}.OwningApplicationCode())
}
