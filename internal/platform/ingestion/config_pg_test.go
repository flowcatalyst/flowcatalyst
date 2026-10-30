//go:build integration

package ingestion_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// The config entries of section ingest, read with the ingestion: the poll
// resolves them, the repository reads them, and the query that finds the
// ingestions naming an entry. The entries are written here straight to the
// table (the operation that writes them through an ingestion is tested with
// the API).

const sftpValue = `{"host":"sftp.example.com","port":22,"directory":"/in","username":"feed","credential":{"name":"sftp-password"},"every":"PT5M","knownHosts":"sftp.example.com ssh-ed25519 AAAA"}`

// refDef is a definition around the source JSON whose target gives the URL
// (so an entry need not).
func refDef(t *testing.T, source string) ingestion.Definition {
	t.Helper()
	return parse(t, definition(source, `{"kind":"none"}`))
}

// putEntry writes the entry of key for the environment's client.
func (e *env) putEntry(t *testing.T, key, value string) {
	t.Helper()
	repo := platformconfig.NewRepository(e.pool)
	cid := string(e.client.ID)
	c, err := repo.FindByCoordinate(context.Background(), e.app.Code, ingestion.ConfigSection, key, platformconfig.ScopeClient, &cid)
	require.NoError(t, err)
	if c == nil {
		c = platformconfig.NewConfig(e.app.Code, ingestion.ConfigSection, key, value)
		c.Scope = platformconfig.ScopeClient
		c.ClientID = &e.client.ID
	}
	c.Value = value
	require.NoError(t, withTx(t, e.uow, func(tx *usecasepgx.DbTx) error { return repo.Persist(context.Background(), c, tx) }))
}

func TestSectionNameIsTheOneThePlatformConfigReserves(t *testing.T) {
	assert.Equal(t, platformconfig.ReservedSection, ingestion.ConfigSection)
}

func TestPoll_ListsAnIngestionOnlyWhileItsEntryIsUsable(t *testing.T) {
	e := newEnv(t)
	inline := e.live(t, "inline", e.sftp(t))
	missing := e.live(t, "missing", refDef(t, `{"kind":"sftp","config":"missing-key"}`))
	invalid := e.live(t, "invalid", refDef(t, `{"kind":"sftp","config":"invalid-key"}`))
	set := e.live(t, "set", refDef(t, `{"kind":"sftp","config":"set-key"}`))
	own := e.live(t, "own-code", refDef(t, `{"kind":"sqs","config":true}`))
	e.putEntry(t, "invalid-key", `{"host":"sftp.example.com","port":0}`)
	e.putEntry(t, "set-key", sftpValue)
	e.putEntry(t, "own-code", `{"region":"eu-west-1","queue":"q"}`)

	res := e.poll(t, "cfg-rt-1", e.t0)
	assert.ElementsMatch(t, []string{inline.ID, set.ID, own.ID}, given(res), "a missing or invalid entry is not listed")

	byID := map[string]ingestion.ListItem{}
	for _, item := range res.Ingestions {
		byID[item.ID] = item
	}
	sftp, ok := byID[set.ID].Definition.Source.(ingestion.Sftp)
	require.True(t, ok, "the runtime is given the effective source")
	assert.Equal(t, "sftp.example.com", sftp.Host)
	assert.Equal(t, "https://hooks.example.com/orders", byID[set.ID].Definition.Target.TargetURL)
	_, isSqs := byID[own.ID].Definition.Source.(ingestion.Sqs)
	assert.True(t, isSqs, "config: true reads the entry named by the ingestion's code")
	for _, item := range res.Ingestions {
		_, unresolved := item.Definition.Source.(ingestion.Configured)
		assert.False(t, unresolved, "a runtime never sees a reference: %s", item.Code)
	}

	assert.Equal(t, 0, e.count(t, `SELECT count(*) FROM ing_leases WHERE ingestion_id = ANY($1)`, []string{missing.ID, invalid.ID}), "and takes no lease")
	assert.Equal(t, 2, e.count(t, `SELECT count(*) FROM ing_leases WHERE ingestion_id = ANY($1)`, []string{inline.ID, set.ID}))

	// The stored definition is still the reference.
	stored := e.mustFind(t, set.ID)
	_, still := stored.Definition.Source.(ingestion.Configured)
	assert.True(t, still)

	// It starts by itself once the entry is valid.
	e.putEntry(t, "missing-key", sftpValue)
	e.putEntry(t, "invalid-key", sftpValue)
	assert.ElementsMatch(t, []string{inline.ID, missing.ID, invalid.ID, set.ID, own.ID}, given(e.poll(t, "cfg-rt-1", e.t0.Add(time.Minute))))
}

func TestPoll_AnEntryIsPerClient(t *testing.T) {
	e := newEnv(t)
	other := newEnv(t)
	i := e.live(t, "per-client", refDef(t, `{"kind":"sftp","config":"shared-name"}`))
	// Same application code and key, another client's entry: not this client's.
	c := platformconfig.NewConfig(e.app.Code, ingestion.ConfigSection, "shared-name", sftpValue)
	c.Scope = platformconfig.ScopeClient
	c.ClientID = &other.client.ID
	require.NoError(t, withTx(t, e.uow, func(tx *usecasepgx.DbTx) error {
		return platformconfig.NewRepository(e.pool).Persist(context.Background(), c, tx)
	}))
	assert.Empty(t, given(e.poll(t, "cfg-rt-2", e.t0)))
	e.putEntry(t, "shared-name", sftpValue)
	assert.Equal(t, []string{i.ID}, given(e.poll(t, "cfg-rt-2", e.t0.Add(time.Minute))))
}

// The fair share counts only what runs: two ingestions whose entries are
// missing do not make a runtime hold more than its share of the two that run.
func TestFairShare_DoesNotCountAnIngestionWhoseEntryIsNotUsable(t *testing.T) {
	e := newEnv(t)
	tf := day(20)
	var running []string
	for _, code := range []string{"run-a", "run-b"} {
		running = append(running, e.live(t, code, refDef(t, `{"kind":"sftp","config":"`+code+`"}`)).ID)
		e.putEntry(t, code, sftpValue)
	}
	e.live(t, "wait-a", refDef(t, `{"kind":"sftp","config":"wait-a"}`))
	e.live(t, "wait-b", refDef(t, `{"kind":"sftp","config":"wait-b"}`))

	assert.ElementsMatch(t, running, given(e.poll(t, "cfgfair-a", tf)), "alone it holds both that run")
	now := tf.Add(time.Minute)
	e.poll(t, "cfgfair-b", now)
	a := given(e.poll(t, "cfgfair-a", now.Add(time.Second)))
	assert.Len(t, a, 1, "two runtimes share the two that run: one each (four would have made it two)")
}

func TestResolveReadsTheEntryOfTheIngestionsClient(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	def := refDef(t, `{"kind":"sftp","config":true}`)

	r, err := e.repo.Resolve(ctx, e.app.Code, e.client.ID, "feed", def)
	require.NoError(t, err)
	assert.Equal(t, ingestion.ConfigMissing, r.Status)
	assert.Equal(t, "feed", r.Key)

	e.putEntry(t, "feed", sftpValue)
	r, err = e.repo.Resolve(ctx, e.app.Code, e.client.ID, "feed", def)
	require.NoError(t, err)
	require.Equal(t, ingestion.ConfigSet, r.Status, r.Problems.String())
	assert.NotNil(t, r.UpdatedAt)
	assert.Equal(t, []string{"sftp-password"}, ingestion.SecretNames(r.Definition.Source))

	// An inline definition reads nothing.
	r, err = e.repo.Resolve(ctx, "no-such-app", e.client.ID, "feed", e.sftp(t))
	require.NoError(t, err)
	assert.Equal(t, ingestion.ConfigInline, r.Status)

	// A row of the section that is a SECRET is not an entry.
	secret := platformconfig.NewConfig(e.app.Code, ingestion.ConfigSection, "sealed", `{}`)
	secret.Scope, secret.ClientID = platformconfig.ScopeClient, &e.client.ID
	secret.ValueType = platformconfig.ValueSecret
	secret.Value = "x"
	// (written as the repository would: not through the encrypting path, which needs a key)
	_, err = e.pool.Exec(ctx, `INSERT INTO app_platform_configs (id, application_code, section, property, scope, client_id, value_type, value, created_at, updated_at)
		VALUES ($1, $2, 'ingest', 'sealed', 'CLIENT', $3, 'SECRET', 'encrypted:x', now(), now())`, secret.ID, e.app.Code, string(e.client.ID))
	require.NoError(t, err)
	r, err = e.repo.Resolve(ctx, e.app.Code, e.client.ID, "x", refDef(t, `{"kind":"sftp","config":"sealed"}`))
	require.NoError(t, err)
	assert.Equal(t, ingestion.ConfigInvalid, r.Status)
}

func TestNamingConfig_LiveOrLatestVersionOfTheSameApplicationAndClient(t *testing.T) {
	e := newEnv(t)
	other := newEnv(t)
	ctx := context.Background()

	byKey := e.live(t, "by-key", refDef(t, `{"kind":"sftp","config":"vendor"}`))
	byCode := e.live(t, "vendor", refDef(t, `{"kind":"sftp","config":true}`))
	quoted := e.live(t, "quoted", refDef(t, `{"kind":"sftp","config":"true"}`))
	inline := e.live(t, "inline", e.sftp(t))
	elsewhere := other.live(t, "elsewhere", refDef(t, `{"kind":"sftp","config":"vendor"}`))
	_ = inline

	// A draft names it, over a live version that is inline.
	draft := e.live(t, "draft-names", e.sftp(t))
	draft = e.mustFind(t, draft.ID)
	draft.SaveVersion(refDef(t, `{"kind":"sftp","config":"vendor"}`), false, nil, nil)
	e.mustSave(t, draft)

	// A version that is neither live nor the latest does not count.
	old := e.live(t, "old-names", refDef(t, `{"kind":"sftp","config":"vendor"}`))
	old = e.mustFind(t, old.ID)
	old.SaveVersion(e.sftp(t), true, nil, nil)
	old.SaveVersion(e.sftp(t), true, nil, nil)
	e.mustSave(t, old)

	got, err := e.repo.NamingConfig(ctx, e.app.ID, e.client.ID, "vendor")
	require.NoError(t, err)
	var ids []string
	for _, i := range got {
		ids = append(ids, i.ID)
	}
	assert.ElementsMatch(t, []string{byKey.ID, byCode.ID, draft.ID}, ids, "by key, by its own code, and by a draft")
	assert.NotContains(t, ids, elsewhere.ID, "another client's")
	assert.NotContains(t, ids, old.ID)

	got, err = e.repo.NamingConfig(ctx, e.app.ID, e.client.ID, "true")
	require.NoError(t, err)
	require.Len(t, got, 1, "the key \"true\" is not config: true")
	assert.Equal(t, quoted.ID, got[0].ID)
}

func TestConfigNamed_TheLiveVersionsKeyElseTheLatests(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	i := e.live(t, "ci", refDef(t, `{"kind":"sftp","config":"live-key"}`))
	i = e.mustFind(t, i.ID)
	i.SaveVersion(refDef(t, `{"kind":"sftp","config":"draft-key"}`), false, nil, nil)
	e.mustSave(t, i)
	i = e.mustFind(t, i.ID)
	_, key, ok, err := e.repo.ConfigNamed(ctx, i)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "live-key", key)

	inline := e.live(t, "ci-inline", e.sftp(t))
	inline = e.mustFind(t, inline.ID)
	inline.SaveVersion(refDef(t, `{"kind":"sftp","config":true}`), false, nil, nil)
	e.mustSave(t, inline)
	_, key, ok, err = e.repo.ConfigNamed(ctx, e.mustFind(t, inline.ID))
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "ci-inline", key)

	plain := e.live(t, "ci-plain", e.sftp(t))
	_, _, ok, err = e.repo.ConfigNamed(ctx, plain)
	require.NoError(t, err)
	assert.False(t, ok)
}

var _ = json.Marshal
