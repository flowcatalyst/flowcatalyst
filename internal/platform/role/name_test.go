package role_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

func TestValidateName(t *testing.T) {
	good := []string{
		"platform:admin", "platform:super-admin", "example:dispatcher",
		"logistics_portal:dashboard:user", // multi-colon: tolerated, see ShortName
		"orders:order.admin", "Orders:Admin", "a1:b2",
	}
	for _, n := range good {
		assert.NoError(t, role.ValidateName(n), n)
	}
	bad := []string{
		"", " ", "example: dispatcher", "example :dispatcher", " example:dispatcher",
		"example:dispatcher ", "example:dis patcher", "example:dispatcher\n", "example:\tdispatcher",
		"example:\u00a0dispatcher", "example:\u200bdispatcher", // nbsp, zero-width space
		"dispatcher", ":dispatcher", "example:", "example::dispatcher", "example:dispatcher:",
		"example:dis;patcher", "example:diß", "exa mple:dispatcher",
	}
	for _, n := range bad {
		err := role.ValidateName(n)
		require.Error(t, err, "%q", n)
		testpgLike(t, err, role.InvalidNameCode, n)
	}
}

func TestValidatePermission(t *testing.T) {
	for _, p := range []string{"platform:*:*:*", "orders:order:create", "platform:iam:user:view", "x"} {
		assert.NoError(t, role.ValidatePermission(p), p)
	}
	for _, p := range []string{"", " ", "orders: order:create", "orders:order:create ", "orders::create", ":a", "a:", "a:b c"} {
		err := role.ValidatePermission(p)
		require.Error(t, err, "%q", p)
		testpgLike(t, err, role.InvalidPermissionCode, p)
	}
}

func TestValidateNames_ReportsOffender(t *testing.T) {
	require.NoError(t, role.ValidateNames([]string{"a:b", "c:d"}))
	err := role.ValidateNames([]string{"a:b", "example: dispatcher"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"example: dispatcher"`)
}

// testpgLike asserts a validation usecase error with the code, whose message
// quotes the offending value.
func testpgLike(t *testing.T, err error, code, value string) {
	t.Helper()
	var ue *usecase.Error
	require.ErrorAs(t, err, &ue, "%q", value)
	assert.Equal(t, usecase.KindValidation, ue.Kind)
	assert.Equal(t, code, ue.Code)
}

func TestName_ScanAndValue(t *testing.T) {
	var n role.Name
	require.NoError(t, n.Scan("a:b"))
	assert.Equal(t, "a:b", n.String())
	require.NoError(t, n.Scan([]byte("c:d")))
	assert.Equal(t, "c:d", n.String())
	v, err := n.Value()
	require.NoError(t, err)
	assert.Equal(t, "c:d", v)
	require.Error(t, n.Scan(42))
}
