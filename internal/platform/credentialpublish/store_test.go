package credentialpublish

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/credentialpublish/credentialpublishtest"
)

const (
	testPath = "/fc/service-accounts/orders/auth-token"
	ourTag   = "SERVICE_ACCOUNT/sac_ours"
)

func newStore(kms string) (Store, *credentialpublishtest.SSM) {
	f := credentialpublishtest.New()
	return NewParameterStore(f, kms), f
}

func TestStorePutCreatesTaggedSecureString(t *testing.T) {
	s, f := newStore("alias/fc")
	require.NoError(t, s.Put(context.Background(), testPath, "v1", ourTag))
	p, ok := f.Get(testPath)
	require.True(t, ok)
	assert.Equal(t, "v1", p.Value)
	assert.Equal(t, "SecureString", string(p.Type))
	assert.Equal(t, "alias/fc", p.KeyID)
	assert.Equal(t, ourTag, p.Tags[TagKey])
}

func TestStorePutWithoutKMSKeyUsesTheDefault(t *testing.T) {
	s, f := newStore("")
	require.NoError(t, s.Put(context.Background(), testPath, "v1", ourTag))
	p, _ := f.Get(testPath)
	assert.Empty(t, p.KeyID)
}

func TestStorePutOverwritesItsOwn(t *testing.T) {
	s, f := newStore("")
	ctx := context.Background()
	require.NoError(t, s.Put(ctx, testPath, "v1", ourTag))
	require.NoError(t, s.Put(ctx, testPath, "v2", ourTag))
	p, _ := f.Get(testPath)
	assert.Equal(t, "v2", p.Value)
	assert.Equal(t, ourTag, p.Tags[TagKey], "the tag survives the overwrite")
}

func TestStorePutRefusesAForeignParameter(t *testing.T) {
	ctx := context.Background()
	for name, tags := range map[string]map[string]string{
		"untagged":        nil,
		"another owner's": {TagKey: "SERVICE_ACCOUNT/sac_other"},
		"another tag":     {"team": "x"},
	} {
		t.Run(name, func(t *testing.T) {
			s, f := newStore("")
			f.Seed(testPath, "theirs", tags)
			err := s.Put(ctx, testPath, "ours", ourTag)
			require.ErrorIs(t, err, ErrNotManaged)
			p, _ := f.Get(testPath)
			assert.Equal(t, "theirs", p.Value)
		})
	}
}

func TestStorePutReturnsAWSFailures(t *testing.T) {
	s, f := newStore("")
	f.FailPath(testPath, credentialpublishtest.Err("AccessDeniedException", "no"))
	err := s.Put(context.Background(), testPath, "v", ourTag)
	require.Error(t, err)
	assert.Equal(t, "AccessDeniedException", ErrorCode(err))
	assert.Equal(t, "AccessDeniedException: no", errorText(err, testPath))
}

func TestStoreDeleteOurs(t *testing.T) {
	s, f := newStore("")
	ctx := context.Background()
	require.NoError(t, s.Put(ctx, testPath, "v", ourTag))
	require.NoError(t, s.Delete(ctx, testPath, ourTag))
	_, ok := f.Get(testPath)
	assert.False(t, ok)
}

func TestStoreDeleteLeavesAForeignParameter(t *testing.T) {
	s, f := newStore("")
	f.Seed(testPath, "theirs", map[string]string{TagKey: "SERVICE_ACCOUNT/sac_other"})
	require.ErrorIs(t, s.Delete(context.Background(), testPath, ourTag), ErrNotManaged)
	_, ok := f.Get(testPath)
	assert.True(t, ok)

	f.Seed(testPath, "untagged", nil)
	require.ErrorIs(t, s.Delete(context.Background(), testPath, ourTag), ErrNotManaged)
	assert.Zero(t, f.Deletes)
}

func TestStoreDeleteMissingIsDone(t *testing.T) {
	s, _ := newStore("")
	assert.NoError(t, s.Delete(context.Background(), testPath, ourTag))
}

func TestErrNotManagedText(t *testing.T) {
	assert.Equal(t, "a parameter FlowCatalyst does not manage already exists at "+testPath,
		errorText(ErrNotManaged, testPath))
}
