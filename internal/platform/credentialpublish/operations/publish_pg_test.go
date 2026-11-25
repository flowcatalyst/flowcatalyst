//go:build integration

package operations_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	authops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/credentialpublish"
	cpops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/credentialpublish/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	saops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

func TestMain(m *testing.M) {
	key, err := encryption.GenerateKey()
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("FLOWCATALYST_APP_KEY", key)
	testpg.RunMain(m)
}

var (
	on  = credentialpublish.Config{Store: credentialpublish.StoreAWSParameterStore, Prefix: "/fc/test/", ServiceAccountPath: credentialpublish.DefaultServiceAccountPath, OAuthClientPath: credentialpublish.DefaultOAuthClientPath}
	off = credentialpublish.Config{}
	yes = true
	no  = false
)

type fixture struct {
	t          *testing.T
	uow        *usecasepgx.UnitOfWork
	accounts   *serviceaccount.Repository
	principals *principal.Repository
	clients    *platformauth.OAuthClientRepo
	apps       *application.Repository
	pubs       *credentialpublish.Repository
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	pool := testpg.Pool(t)
	return fixture{
		t: t, uow: testpg.NewUoW(t),
		accounts:   serviceaccount.NewRepository(pool),
		principals: principal.NewRepository(pool),
		clients:    platformauth.NewRepository(pool).OAuthClients,
		apps:       application.NewRepository(pool),
		pubs:       credentialpublish.NewRepository(pool),
	}
}

func (f fixture) createAccount(cfg credentialpublish.Config, cmd saops.CreateCommand) (saops.CreateWithCredentialsResult, error) {
	pool := testpg.Pool(f.t)
	return usecaseop.RunTx(testpg.AnchorCtx(), f.uow,
		saops.CreateServiceAccountWithCredentials(f.accounts, f.principals, f.clients,
			client.NewRepository(pool), principal.NewClientAccessGrantRepo(pool), cfg),
		cmd, testpg.TestEC())
}

func (f fixture) mustCreateAccount(cfg credentialpublish.Config, code string, publish *bool) saops.CreateWithCredentialsResult {
	f.t.Helper()
	res, err := f.createAccount(cfg, saops.CreateCommand{Code: code, Name: code, PublishCredentials: publish})
	require.NoError(f.t, err)
	return res
}

func (f fixture) createClient(cfg credentialpublish.Config, cmd authops.CreateOAuthClientCommand) (authops.OAuthClientCreated, error) {
	return usecaseop.Run(testpg.AnchorCtx(), f.uow, authops.CreateOAuthClient(f.clients, cfg), cmd, testpg.TestEC())
}

func (f fixture) rotate(rowID string) {
	f.t.Helper()
	_, err := usecaseop.Run(testpg.AnchorCtx(), f.uow, authops.RotateOAuthClientSecret(f.clients),
		authops.RotateOAuthClientSecretCommand{ID: rowID}, testpg.TestEC())
	require.NoError(f.t, err)
}

func (f fixture) enable(cfg credentialpublish.Config, t credentialpublish.OwnerType, id string, base *string) error {
	_, err := usecaseop.Run(testpg.AnchorCtx(), f.uow,
		cpops.EnableCredentialPublishing(cfg, f.accounts, f.principals, f.clients, f.apps),
		cpops.EnableCommand{OwnerType: t, OwnerID: id, BasePath: base}, testpg.TestEC())
	return err
}

func (f fixture) disable(t credentialpublish.OwnerType, id string) error {
	_, err := usecaseop.Run(testpg.AnchorCtx(), f.uow,
		cpops.DisableCredentialPublishing(f.accounts, f.clients, f.pubs),
		cpops.DisableCommand{OwnerType: t, OwnerID: id}, testpg.TestEC())
	return err
}

func saOwner(id string) credentialpublish.Owner {
	return credentialpublish.Owner{Type: credentialpublish.OwnerServiceAccount, ID: id}
}
func ocOwner(id string) credentialpublish.Owner {
	return credentialpublish.Owner{Type: credentialpublish.OwnerOAuthClient, ID: id}
}

// items is the owner's items by credential, and its publication (nil when off).
func (f fixture) items(o credentialpublish.Owner) (*credentialpublish.Publication, map[string]credentialpublish.Item) {
	f.t.Helper()
	pub, items, err := f.pubs.Status(context.Background(), o)
	require.NoError(f.t, err)
	out := map[string]credentialpublish.Item{}
	for _, it := range items {
		out[it.Credential] = it
	}
	return pub, out
}

// pendingValue is the decrypted pending_value of one item ("" when none).
func (f fixture) pendingValue(o credentialpublish.Owner, credential string) string {
	f.t.Helper()
	var ct *string
	require.NoError(f.t, testpg.Pool(f.t).QueryRow(context.Background(),
		`SELECT pending_value FROM iam_credential_publication_items WHERE owner_type=$1 AND owner_id=$2 AND credential=$3`,
		string(o.Type), o.ID, credential).Scan(&ct))
	if ct == nil {
		return ""
	}
	pt, err := encryption.MustFromEnv().Decrypt(*ct)
	require.NoError(f.t, err)
	return pt
}

// markPublished stands in for the worker having written every item.
func (f fixture) markPublished(o credentialpublish.Owner) {
	f.t.Helper()
	_, err := testpg.Pool(f.t).Exec(context.Background(),
		`UPDATE iam_credential_publication_items SET state='PUBLISHED', pending_value=NULL, attempts=0
		  WHERE owner_type=$1 AND owner_id=$2 AND state IN ('PENDING_PUT','PENDING_DELETE')`, string(o.Type), o.ID)
	require.NoError(f.t, err)
}

func states(items map[string]credentialpublish.Item) map[string]string {
	out := map[string]string{}
	for k, v := range items {
		out[k] = v.State
	}
	return out
}

func (f fixture) clientRowID(res saops.CreateWithCredentialsResult) string {
	return res.OAuthClientRowID
}

func TestCreateAccountPublishesAllFour(t *testing.T) {
	f := newFixture(t)
	res := f.mustCreateAccount(on, "cp-create-on", &yes)
	pub, items := f.items(saOwner(res.ServiceAccount.ID))
	require.NotNil(t, pub)
	assert.Equal(t, "/fc/test/service-accounts/cp-create-on/", pub.BasePath)
	assert.Equal(t, map[string]string{
		"client-id": "PENDING_PUT", "client-secret": "PENDING_PUT", "auth-token": "PENDING_PUT", "signing-secret": "PENDING_PUT",
	}, states(items))
	assert.Equal(t, "/fc/test/service-accounts/cp-create-on/client-secret", items["client-secret"].Path)
	// The secret exists nowhere else: it travels encrypted in the queue.
	assert.Equal(t, res.OAuthClientSecret, f.pendingValue(saOwner(res.ServiceAccount.ID), "client-secret"))
	assert.Empty(t, f.pendingValue(saOwner(res.ServiceAccount.ID), "auth-token"), "readable credentials are read at publish time")
	// The account's own client has no publication of its own.
	pub, _ = f.items(ocOwner(res.OAuthClientRowID))
	assert.Nil(t, pub)
}

func TestCreateAccountDefaultsAndRefusals(t *testing.T) {
	f := newFixture(t)

	t.Run("nil with the default off queues nothing", func(t *testing.T) {
		res := f.mustCreateAccount(on, "cp-create-nil-off", nil)
		pub, items := f.items(saOwner(res.ServiceAccount.ID))
		assert.Nil(t, pub)
		assert.Empty(t, items)
	})
	t.Run("nil with the default on publishes", func(t *testing.T) {
		cfg := on
		cfg.DefaultOn = true
		res := f.mustCreateAccount(cfg, "cp-create-nil-on", nil)
		pub, items := f.items(saOwner(res.ServiceAccount.ID))
		assert.NotNil(t, pub)
		assert.Len(t, items, 4)
	})
	t.Run("false overrides the default", func(t *testing.T) {
		cfg := on
		cfg.DefaultOn = true
		res := f.mustCreateAccount(cfg, "cp-create-false", &no)
		pub, _ := f.items(saOwner(res.ServiceAccount.ID))
		assert.Nil(t, pub)
	})
	t.Run("true while unavailable is refused and nothing is written", func(t *testing.T) {
		_, err := f.createAccount(off, saops.CreateCommand{Code: "cp-create-unavail", Name: "x", PublishCredentials: &yes})
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, credentialpublish.CodeUnavailable)
		sa, err := f.accounts.FindByCode(context.Background(), "cp-create-unavail")
		require.NoError(t, err)
		assert.Nil(t, sa)
	})
	t.Run("a path outside the prefix is refused and nothing is written", func(t *testing.T) {
		bad := "/elsewhere/"
		_, err := f.createAccount(on, saops.CreateCommand{Code: "cp-create-badpath", Name: "x", PublishCredentials: &yes, PublishBasePath: &bad})
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, credentialpublish.CodePathInvalid)
		sa, err := f.accounts.FindByCode(context.Background(), "cp-create-badpath")
		require.NoError(t, err)
		assert.Nil(t, sa)
	})
	t.Run("a custom path under the prefix is used", func(t *testing.T) {
		custom := "/fc/test/custom/mine/"
		res, err := f.createAccount(on, saops.CreateCommand{Code: "cp-create-custom", Name: "x", PublishCredentials: &yes, PublishBasePath: &custom})
		require.NoError(t, err)
		pub, items := f.items(saOwner(res.ServiceAccount.ID))
		require.NotNil(t, pub)
		assert.Equal(t, custom, pub.BasePath)
		assert.Equal(t, custom+"auth-token", items["auth-token"].Path)
	})
}

func TestProvisionForApplicationPublishesUnderAppSegment(t *testing.T) {
	f := newFixture(t)
	ev, err := usecaseop.Run(testpg.AnchorCtx(), f.uow, appops.CreateApplication(f.apps),
		appops.CreateCommand{Code: "orders", Name: "Orders"}, testpg.TestEC())
	require.NoError(t, err)
	res, err := usecaseop.RunTx(testpg.AnchorCtx(), f.uow,
		appops.ProvisionServiceAccount(f.apps, f.accounts, f.principals, f.clients, on),
		appops.ProvisionServiceAccountCommand{ApplicationID: ev.ApplicationID, PublishCredentials: &yes}, testpg.TestEC())
	require.NoError(t, err)
	pub, items := f.items(saOwner(res.ServiceAccountID))
	require.NotNil(t, pub)
	assert.Equal(t, "/fc/test/service-accounts/app.orders/", pub.BasePath)
	assert.Len(t, items, 4)
	assert.Equal(t, res.OAuthClientSecret, f.pendingValue(saOwner(res.ServiceAccountID), "client-secret"))
}

func TestRegenerateRequeuesOnlyWhenPublishing(t *testing.T) {
	f := newFixture(t)
	regen := func(id string) {
		t.Helper()
		_, err := usecaseop.Run(testpg.AnchorCtx(), f.uow, saops.RegenerateAuthToken(f.accounts),
			saops.RegenerateAuthTokenCommand{ServiceAccountID: id}, testpg.TestEC())
		require.NoError(t, err)
		_, err = usecaseop.Run(testpg.AnchorCtx(), f.uow, saops.RegenerateSigningSecret(f.accounts),
			saops.RegenerateSigningSecretCommand{ServiceAccountID: id}, testpg.TestEC())
		require.NoError(t, err)
	}

	pubRes := f.mustCreateAccount(on, "cp-regen-on", &yes)
	o := saOwner(pubRes.ServiceAccount.ID)
	f.markPublished(o)
	regen(o.ID)
	_, items := f.items(o)
	assert.Equal(t, "PENDING_PUT", items["auth-token"].State)
	assert.Equal(t, "PENDING_PUT", items["signing-secret"].State)
	assert.Equal(t, "PUBLISHED", items["client-id"].State, "only what changed is queued")

	plain := f.mustCreateAccount(on, "cp-regen-off", &no)
	regen(plain.ServiceAccount.ID)
	pub, items := f.items(saOwner(plain.ServiceAccount.ID))
	assert.Nil(t, pub)
	assert.Empty(t, items)
}

func TestUpdateWebhookCredentialsQueuesPutAndDelete(t *testing.T) {
	f := newFixture(t)
	res := f.mustCreateAccount(on, "cp-update", &yes)
	o := saOwner(res.ServiceAccount.ID)
	f.markPublished(o)
	pool := testpg.Pool(t)
	update := func(creds serviceaccount.WebhookCredentials) {
		t.Helper()
		_, err := usecaseop.RunTx(testpg.AnchorCtx(), f.uow,
			saops.UpdateServiceAccount(f.accounts, f.principals, client.NewRepository(pool), principal.NewClientAccessGrantRepo(pool)),
			saops.UpdateCommand{ID: o.ID, WebhookCredentials: &creds}, testpg.TestEC())
		require.NoError(t, err)
	}
	token, secret := "fc_changedtoken", res.SigningSecret

	update(serviceaccount.WebhookCredentials{AuthType: serviceaccount.AuthBearer, Token: &token, SigningSecret: &secret})
	_, items := f.items(o)
	assert.Equal(t, "PENDING_PUT", items["auth-token"].State, "a changed token is put again")
	assert.Equal(t, "PUBLISHED", items["signing-secret"].State, "an unchanged secret is left alone")

	f.markPublished(o)
	update(serviceaccount.WebhookCredentials{AuthType: serviceaccount.AuthBearer, Token: &token, SigningSecret: &secret})
	_, items = f.items(o)
	assert.Equal(t, "PUBLISHED", items["auth-token"].State, "an identical update queues nothing")

	update(serviceaccount.WebhookCredentials{AuthType: serviceaccount.AuthHMAC, SigningSecret: &secret})
	_, items = f.items(o)
	assert.Equal(t, "PENDING_DELETE", items["auth-token"].State, "a removed token is deleted")
	assert.Equal(t, "PUBLISHED", items["signing-secret"].State)
}

func TestCreateOAuthClient(t *testing.T) {
	f := newFixture(t)
	conf := func(extra func(*authops.CreateOAuthClientCommand)) authops.CreateOAuthClientCommand {
		c := authops.CreateOAuthClientCommand{ClientName: "cp client", ClientType: "CONFIDENTIAL"}
		extra(&c)
		return c
	}

	t.Run("a stand-alone confidential client enables publishing", func(t *testing.T) {
		ev, err := f.createClient(on, conf(func(c *authops.CreateOAuthClientCommand) { c.PublishCredentials = &yes }))
		require.NoError(t, err)
		pub, items := f.items(ocOwner(ev.OAuthClientID))
		require.NotNil(t, pub)
		assert.Equal(t, "/fc/test/oauth-clients/"+ev.ClientID+"/", pub.BasePath)
		assert.Equal(t, map[string]string{"client-id": "PENDING_PUT", "client-secret": "PENDING_PUT"}, states(items))
		secret, ok := authops.PopStashedSecret(ev.OAuthClientID)
		require.True(t, ok)
		assert.Equal(t, secret, f.pendingValue(ocOwner(ev.OAuthClientID), "client-secret"))
	})
	t.Run("a client of a service account's principal does not enable; explicit true is refused", func(t *testing.T) {
		acct := f.mustCreateAccount(on, "cp-oc-linked", &yes)
		pid := acct.PrincipalID
		cfg := on
		cfg.DefaultOn = true
		ev, err := f.createClient(cfg, conf(func(c *authops.CreateOAuthClientCommand) { c.PrincipalID = &pid }))
		require.NoError(t, err)
		pub, _ := f.items(ocOwner(ev.OAuthClientID))
		assert.Nil(t, pub, "it publishes with its account")

		_, err = f.createClient(on, conf(func(c *authops.CreateOAuthClientCommand) { c.PrincipalID = &pid; c.PublishCredentials = &yes }))
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, credentialpublish.CodeLinked)
	})
	t.Run("a public client refuses true and ignores the default", func(t *testing.T) {
		_, err := f.createClient(on, authops.CreateOAuthClientCommand{ClientName: "pub", ClientType: "PUBLIC", PublishCredentials: &yes})
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, credentialpublish.CodeNotConfidential)
		cfg := on
		cfg.DefaultOn = true
		ev, err := f.createClient(cfg, authops.CreateOAuthClientCommand{ClientName: "pub", ClientType: "PUBLIC"})
		require.NoError(t, err)
		pub, _ := f.items(ocOwner(ev.OAuthClientID))
		assert.Nil(t, pub)
	})
	t.Run("true while unavailable is refused", func(t *testing.T) {
		_, err := f.createClient(off, conf(func(c *authops.CreateOAuthClientCommand) { c.PublishCredentials = &yes }))
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, credentialpublish.CodeUnavailable)
	})
}

func TestRotateQueuesTheSecretUnderTheRightOwner(t *testing.T) {
	f := newFixture(t)

	t.Run("a linked client's secret goes to its account", func(t *testing.T) {
		acct := f.mustCreateAccount(on, "cp-rot-linked", &yes)
		o := saOwner(acct.ServiceAccount.ID)
		f.markPublished(o)
		f.rotate(acct.OAuthClientRowID)
		secret, ok := authops.PopStashedSecret(acct.OAuthClientRowID)
		require.True(t, ok)
		_, items := f.items(o)
		assert.Equal(t, "PENDING_PUT", items["client-secret"].State)
		assert.Equal(t, secret, f.pendingValue(o, "client-secret"))
		assert.Equal(t, "PUBLISHED", items["client-id"].State)
		pub, own := f.items(ocOwner(acct.OAuthClientRowID))
		assert.Nil(t, pub)
		assert.Empty(t, own, "the linked client has no items of its own")
	})
	t.Run("a stand-alone client's secret goes under itself", func(t *testing.T) {
		ev, err := f.createClient(on, authops.CreateOAuthClientCommand{ClientName: "c", ClientType: "CONFIDENTIAL", PublishCredentials: &yes})
		require.NoError(t, err)
		o := ocOwner(ev.OAuthClientID)
		f.markPublished(o)
		f.rotate(ev.OAuthClientID)
		secret, _ := authops.PopStashedSecret(ev.OAuthClientID)
		_, items := f.items(o)
		assert.Equal(t, "PENDING_PUT", items["client-secret"].State)
		assert.Equal(t, secret, f.pendingValue(o, "client-secret"))
	})
	t.Run("a client that does not publish queues nothing", func(t *testing.T) {
		ev, err := f.createClient(on, authops.CreateOAuthClientCommand{ClientName: "c", ClientType: "CONFIDENTIAL"})
		require.NoError(t, err)
		f.rotate(ev.OAuthClientID)
		pub, items := f.items(ocOwner(ev.OAuthClientID))
		assert.Nil(t, pub)
		assert.Empty(t, items)
	})
}

func TestDeleteQueuesRemoval(t *testing.T) {
	f := newFixture(t)

	t.Run("a service account", func(t *testing.T) {
		acct := f.mustCreateAccount(on, "cp-del-sa", &yes)
		o := saOwner(acct.ServiceAccount.ID)
		f.markPublished(o)
		_, err := usecaseop.Run(testpg.AnchorCtx(), f.uow, saops.DeleteServiceAccount(f.accounts),
			saops.DeleteCommand{ID: o.ID}, testpg.TestEC())
		require.NoError(t, err)
		pub, items := f.items(o)
		assert.Nil(t, pub, "the publication goes with the owner")
		assert.Equal(t, map[string]string{
			"client-id": "PENDING_DELETE", "client-secret": "PENDING_DELETE", "auth-token": "PENDING_DELETE", "signing-secret": "PENDING_DELETE",
		}, states(items), "the items outlive it until the worker has deleted the parameters")
	})
	t.Run("a stand-alone client", func(t *testing.T) {
		ev, err := f.createClient(on, authops.CreateOAuthClientCommand{ClientName: "c", ClientType: "CONFIDENTIAL", PublishCredentials: &yes})
		require.NoError(t, err)
		o := ocOwner(ev.OAuthClientID)
		f.markPublished(o)
		_, err = usecaseop.Run(testpg.AnchorCtx(), f.uow, authops.DeleteOAuthClient(f.clients),
			authops.DeleteOAuthClientCommand{ID: o.ID}, testpg.TestEC())
		require.NoError(t, err)
		pub, items := f.items(o)
		assert.Nil(t, pub)
		assert.Equal(t, map[string]string{"client-id": "PENDING_DELETE", "client-secret": "PENDING_DELETE"}, states(items))
	})
}

func TestEnableLaterThenRotatePublishesTheSecret(t *testing.T) {
	f := newFixture(t)
	acct := f.mustCreateAccount(on, "cp-enable-later", &no)
	o := saOwner(acct.ServiceAccount.ID)
	require.NoError(t, f.enable(on, o.Type, o.ID, nil))
	pub, items := f.items(o)
	require.NotNil(t, pub)
	assert.Equal(t, map[string]string{
		"client-id": "PENDING_PUT", "auth-token": "PENDING_PUT", "signing-secret": "PENDING_PUT", "client-secret": "NOT_PUBLISHABLE",
	}, states(items))
	assert.Empty(t, f.pendingValue(o, "client-secret"))

	f.rotate(acct.OAuthClientRowID)
	secret, _ := authops.PopStashedSecret(acct.OAuthClientRowID)
	_, items = f.items(o)
	assert.Equal(t, "PENDING_PUT", items["client-secret"].State)
	assert.Equal(t, secret, f.pendingValue(o, "client-secret"))

	// A second enable is refused and changes nothing.
	err := f.enable(on, o.Type, o.ID, nil)
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, credentialpublish.CodeAlreadyEnabled)
}

func TestEnableStandAloneClientQueuesIdAndUnavailableSecret(t *testing.T) {
	f := newFixture(t)
	ev, err := f.createClient(on, authops.CreateOAuthClientCommand{ClientName: "c", ClientType: "CONFIDENTIAL"})
	require.NoError(t, err)
	require.NoError(t, f.enable(on, credentialpublish.OwnerOAuthClient, ev.OAuthClientID, nil))
	_, items := f.items(ocOwner(ev.OAuthClientID))
	assert.Equal(t, map[string]string{"client-id": "PENDING_PUT", "client-secret": "NOT_PUBLISHABLE"}, states(items))
}

func TestDisableThenEnableWhileDeletesPending(t *testing.T) {
	f := newFixture(t)
	acct := f.mustCreateAccount(on, "cp-disable", &yes)
	o := saOwner(acct.ServiceAccount.ID)
	f.markPublished(o)

	require.NoError(t, f.disable(o.Type, o.ID))
	pub, items := f.items(o)
	assert.Nil(t, pub)
	for c, it := range items {
		assert.Equal(t, "PENDING_DELETE", it.State, c)
	}
	err := f.enable(on, o.Type, o.ID, nil)
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, credentialpublish.CodeRemovalPending)
	err = f.disable(o.Type, o.ID)
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, credentialpublish.CodeNotEnabled)
}

func TestEnableRefusals(t *testing.T) {
	f := newFixture(t)

	t.Run("not configured", func(t *testing.T) {
		acct := f.mustCreateAccount(on, "cp-ref-unavail", &no)
		err := f.enable(off, credentialpublish.OwnerServiceAccount, acct.ServiceAccount.ID, nil)
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, credentialpublish.CodeUnavailable)
	})
	t.Run("unknown owners are not found", func(t *testing.T) {
		err := f.enable(on, credentialpublish.OwnerServiceAccount, "sac_nope", nil)
		testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "ServiceAccount_NOT_FOUND")
		err = f.enable(on, credentialpublish.OwnerOAuthClient, "oac_nope", nil)
		testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "OAuthClient_NOT_FOUND")
	})
	t.Run("a public client", func(t *testing.T) {
		ev, err := f.createClient(on, authops.CreateOAuthClientCommand{ClientName: "p", ClientType: "PUBLIC"})
		require.NoError(t, err)
		err = f.enable(on, credentialpublish.OwnerOAuthClient, ev.OAuthClientID, nil)
		testpg.RequireUsecaseError(t, err, usecase.KindConflict, credentialpublish.CodeNotConfidential)
	})
	t.Run("a client linked to a service account", func(t *testing.T) {
		acct := f.mustCreateAccount(on, "cp-ref-linked", &no)
		err := f.enable(on, credentialpublish.OwnerOAuthClient, acct.OAuthClientRowID, nil)
		testpg.RequireUsecaseError(t, err, usecase.KindConflict, credentialpublish.CodeLinked)
		err = f.disable(credentialpublish.OwnerOAuthClient, acct.OAuthClientRowID)
		testpg.RequireUsecaseError(t, err, usecase.KindConflict, credentialpublish.CodeLinked)
	})
	t.Run("a custom path outside the prefix", func(t *testing.T) {
		acct := f.mustCreateAccount(on, "cp-ref-path", &no)
		bad := "/other/"
		err := f.enable(on, credentialpublish.OwnerServiceAccount, acct.ServiceAccount.ID, &bad)
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, credentialpublish.CodePathInvalid)
		pub, _ := f.items(saOwner(acct.ServiceAccount.ID))
		assert.Nil(t, pub)
	})
}

func TestRetryMakesWaitingItemsDue(t *testing.T) {
	f := newFixture(t)
	acct := f.mustCreateAccount(on, "cp-retry", &yes)
	o := saOwner(acct.ServiceAccount.ID)
	_, err := testpg.Pool(t).Exec(context.Background(),
		`UPDATE iam_credential_publication_items SET next_attempt_at = now() + interval '1 hour', attempts = 3 WHERE owner_type=$1 AND owner_id=$2`,
		string(o.Type), o.ID)
	require.NoError(t, err)
	require.NoError(t, cpops.RetryCredentialPublishing(context.Background(), f.pubs, o))
	_, items := f.items(o)
	for c, it := range items {
		require.NotNil(t, it.NextAttemptAt, c)
		assert.Equal(t, 3, it.Attempts, "attempts are kept")
	}
	var due int
	require.NoError(t, testpg.Pool(t).QueryRow(context.Background(),
		`SELECT count(*) FROM iam_credential_publication_items WHERE owner_type=$1 AND owner_id=$2 AND next_attempt_at <= now()`,
		string(o.Type), o.ID).Scan(&due))
	assert.Equal(t, 4, due)
}
