//go:build integration

package platformconfig

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func storedValueOf(t *testing.T, id string) string {
	t.Helper()
	var v string
	require.NoError(t, testpg.Pool(t).QueryRow(context.Background(), `SELECT value FROM app_platform_configs WHERE id = $1`, id).Scan(&v))
	return v
}

func insertSecretRow(t *testing.T, id, app, value string) {
	t.Helper()
	_, err := testpg.Pool(t).Exec(context.Background(),
		`INSERT INTO app_platform_configs (id, application_code, section, property, scope, value_type, value, created_at, updated_at)
		 VALUES ($1, $2, 's', 'p', 'GLOBAL', 'SECRET', $3, now(), now())`, id, app, value)
	require.NoError(t, err)
}

func newKeyedRepo(t *testing.T) (*Repository, *encryption.Service) {
	t.Helper()
	key, _ := encryption.GenerateKey()
	enc, _ := encryption.New(key)
	return NewRepository(testpg.Pool(t)).WithEncryption(enc), enc
}

// A secret's ciphertext is bound to its row: it reveals there, and copied
// into another row (another application's) it does not.
func TestSecretAAD_BoundToItsRow(t *testing.T) {
	ctx := context.Background()
	repo, _ := newKeyedRepo(t)
	c := NewConfig("pcaad-a", "s", "p", "s3cr3t")
	c.ValueType = ValueSecret
	stored, err := repo.storedValue(c)
	require.NoError(t, err)
	insertSecretRow(t, "cfg_pcaad_a00001", "pcaad-a", stored)
	insertSecretRow(t, "cfg_pcaad_b00001", "pcaad-b", stored)

	own, err := repo.FindByID(ctx, "cfg_pcaad_a00001")
	require.NoError(t, err)
	got, err := repo.Reveal(ctx, own)
	require.NoError(t, err)
	assert.Equal(t, "s3cr3t", got)

	copied, err := repo.FindByID(ctx, "cfg_pcaad_b00001")
	require.NoError(t, err)
	_, err = repo.Reveal(ctx, copied)
	require.Error(t, err, "a ciphertext copied to another application's row does not open")
}

// A secret sealed before binding (no aad) is still read, and the read repairs
// it: it is re-encrypted bound to its row. Start-up conversion does the same
// for every such row, and for legacy plaintext.
func TestSecretAAD_LegacyFallbackAndRepair(t *testing.T) {
	ctx := context.Background()
	repo, enc := newKeyedRepo(t)
	unbound, err := enc.Encrypt("old-sealed")
	require.NoError(t, err)
	insertSecretRow(t, "cfg_pcaad_l00001", "pcaad-legacy", EncryptedPrefix+unbound)
	insertSecretRow(t, "cfg_pcaad_l00002", "pcaad-legacy2", EncryptedPrefix+unbound)
	insertSecretRow(t, "cfg_pcaad_l00003", "pcaad-legacy3", "old-plain")

	c, err := repo.FindByID(ctx, "cfg_pcaad_l00001")
	require.NoError(t, err)
	got, err := repo.Reveal(ctx, c)
	require.NoError(t, err)
	assert.Equal(t, "old-sealed", got)
	repaired := storedValueOf(t, "cfg_pcaad_l00001")
	assert.NotEqual(t, EncryptedPrefix+unbound, repaired, "the read re-encrypted it")
	_, bound, err := enc.DecryptBound(repaired, secretAAD("pcaad-legacy", "s", "p", nil))
	require.NoError(t, err)
	assert.True(t, bound)

	n, err := repo.EncryptLegacySecrets(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, 2)
	for id, app := range map[string]string{"cfg_pcaad_l00002": "pcaad-legacy2", "cfg_pcaad_l00003": "pcaad-legacy3"} {
		raw := storedValueOf(t, id)
		require.True(t, strings.HasPrefix(raw, EncryptedPrefix), id)
		pt, bound, err := enc.DecryptBound(raw, secretAAD(app, "s", "p", nil))
		require.NoError(t, err, id)
		assert.True(t, bound, id)
		assert.Contains(t, []string{"old-sealed", "old-plain"}, pt)
	}
	before := storedValueOf(t, "cfg_pcaad_l00002")
	_, err = repo.EncryptLegacySecrets(ctx)
	require.NoError(t, err)
	assert.Equal(t, before, storedValueOf(t, "cfg_pcaad_l00002"), "a converted row is left alone")
}

// The conversion replaces a value only while it is still the one it read: a
// value set meanwhile (here: the stored value changed after the read) is not
// overwritten.
func TestRebind_CompareAndSet(t *testing.T) {
	ctx := context.Background()
	repo, _ := newKeyedRepo(t)
	insertSecretRow(t, "cfg_pccas_000001", "pccas", "read-by-the-conversion")
	_, err := testpg.Pool(t).Exec(ctx, `UPDATE app_platform_configs SET value = 'set-meanwhile' WHERE id = 'cfg_pccas_000001'`)
	require.NoError(t, err)

	changed, err := repo.rebind(ctx, "cfg_pccas_000001", "pccas", "s", "p", nil, "read-by-the-conversion", "read-by-the-conversion")
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, "set-meanwhile", storedValueOf(t, "cfg_pccas_000001"), "the newer value survives")
}

// Without a key, start-up does not fail on SECRET rows: it logs them, and every
// SECRET read refuses (legacy plaintext included).
func TestNoKey_StartsAndFailsClosed(t *testing.T) {
	ctx := context.Background()
	repo := NewRepository(testpg.Pool(t)).WithEncryption(nil)
	insertSecretRow(t, "cfg_pcnokey00001", "pcnokey", "plain-legacy")
	n, err := repo.EncryptLegacySecrets(ctx)
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.Equal(t, "plain-legacy", storedValueOf(t, "cfg_pcnokey00001"))
	c, err := repo.FindByID(ctx, "cfg_pcnokey00001")
	require.NoError(t, err)
	_, err = repo.Reveal(ctx, c)
	require.ErrorIs(t, err, ErrNoEncryptionKey)
}
