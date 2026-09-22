package server

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
)

// The 2026-09-22 ruling, pinned: the connection's service account signs a
// subscription's deliveries; the application's oldest active account is only
// the fallback for a job with no connection to name one; an explicitly
// named account that cannot sign is declined WITH A REASON, never silently
// swapped for another.

type credsWorld struct {
	subs  map[string]*subscription.Subscription
	conns map[string]*connection.Connection
	apps  map[string]*application.Application
	bySA  map[string]serviceaccount.OutboundCreds
	byApp map[string]serviceaccount.OutboundCreds
	// calls records which credential lookups ran, so a test can assert that
	// the fallback was NOT consulted.
	calls []string
}

func (w *credsWorld) deps() deliveryCredsDeps {
	return deliveryCredsDeps{
		subscription: func(_ context.Context, id string) (*subscription.Subscription, error) {
			return w.subs[id], nil
		},
		connection: func(_ context.Context, id string) (*connection.Connection, error) {
			return w.conns[id], nil
		},
		application: func(_ context.Context, code string) (*application.Application, error) {
			return w.apps[code], nil
		},
		byServiceAccountID: func(_ context.Context, id string) (serviceaccount.OutboundCreds, error) {
			w.calls = append(w.calls, "sa:"+id)
			c, ok := w.bySA[id]
			if !ok {
				return serviceaccount.OutboundCreds{Reason: "service account " + id + " does not exist"}, nil
			}
			return c, nil
		},
		byApplicationID: func(_ context.Context, id string) (serviceaccount.OutboundCreds, error) {
			w.calls = append(w.calls, "app:"+id)
			return w.byApp[id], nil
		},
	}
}

// A world with one application ("value") owning SA "sa-app", one connection
// ("cnn-value") naming SA "sa-conn", and one subscription on that
// connection whose applicationCode is "value". The two accounts have
// DIFFERENT secrets, so which one signs is observable.
func newCredsWorld() *credsWorld {
	return &credsWorld{
		subs: map[string]*subscription.Subscription{
			"sub-1": {ID: "sub-1", Code: "user-logged-in", ApplicationCode: new("value"), ConnectionID: new("cnn-1")},
		},
		conns: map[string]*connection.Connection{
			"cnn-1": {ID: "cnn-1", Code: "cnn-value", ServiceAccountID: "sa-conn"},
		},
		apps: map[string]*application.Application{
			"value": {ID: "app-value", Code: "value"},
		},
		bySA: map[string]serviceaccount.OutboundCreds{
			"sa-conn": {SigningSecret: "secret-from-connection"},
			"sa-sub":  {SigningSecret: "secret-from-subscription"},
		},
		byApp: map[string]serviceaccount.OutboundCreds{
			"app-value": {SigningSecret: "secret-from-application"},
		},
	}
}

func jobFor(sub, code string) *dispatchjob.DispatchJob {
	j := &dispatchjob.DispatchJob{Code: code}
	if sub != "" {
		j.SubscriptionID = new(sub)
	}
	return j
}

// The owner's exact incident: the connection's account holds the secret the
// subscriber verifies with; the application's oldest account holds another.
func TestDeliveryCredsConnectionAccountSigns(t *testing.T) {
	w := newCredsWorld()
	creds, err := newDeliveryCredsResolver(w.deps())(context.Background(), jobFor("sub-1", "user-logged-in"))
	require.NoError(t, err)
	assert.Equal(t, "secret-from-connection", creds.SigningSecret,
		"the connection's service account signs — not the application's oldest account (the 2026-09-19 rule this reverses)")
	assert.Empty(t, creds.Reason)
	assert.NotContains(t, w.calls, "app:app-value", "the application fallback must not even be consulted")
}

// A subscription that names its own account overrides its connection's.
func TestDeliveryCredsSubscriptionAccountOverridesConnection(t *testing.T) {
	w := newCredsWorld()
	w.subs["sub-1"].ServiceAccountID = new("sa-sub")
	creds, err := newDeliveryCredsResolver(w.deps())(context.Background(), jobFor("sub-1", "user-logged-in"))
	require.NoError(t, err)
	assert.Equal(t, "secret-from-subscription", creds.SigningSecret)
	assert.NotContains(t, w.calls, "sa:sa-conn")
}

// An explicitly named account that cannot sign is declined with a reason
// naming who configured it — never swapped for the application's account.
func TestDeliveryCredsNamedAccountThatCannotSignIsDeclinedNotReplaced(t *testing.T) {
	cases := map[string]serviceaccount.OutboundCreds{
		"inactive":       {Reason: "service account sa-conn is inactive"},
		"no credentials": {},
	}
	for name, saCreds := range cases {
		t.Run(name, func(t *testing.T) {
			w := newCredsWorld()
			w.bySA["sa-conn"] = saCreds
			creds, err := newDeliveryCredsResolver(w.deps())(context.Background(), jobFor("sub-1", "user-logged-in"))
			require.NoError(t, err)
			assert.True(t, creds.Empty(), "must deliver bare, not with the application's secret")
			assert.Contains(t, creds.Reason, "connection cnn-value", "the reason names what configured the account")
			assert.NotContains(t, w.calls, "app:app-value", "no silent fallback to the application's account")
		})
	}
	w := newCredsWorld()
	delete(w.bySA, "sa-conn")
	creds, err := newDeliveryCredsResolver(w.deps())(context.Background(), jobFor("sub-1", "user-logged-in"))
	require.NoError(t, err)
	assert.Contains(t, creds.Reason, "connection cnn-value: service account sa-conn does not exist")
}

// No connection account to name → the application's oldest active account,
// through the subscription's applicationCode.
func TestDeliveryCredsFallsBackToApplicationWhenNothingNamesAnAccount(t *testing.T) {
	w := newCredsWorld()
	w.conns["cnn-1"].ServiceAccountID = ""
	creds, err := newDeliveryCredsResolver(w.deps())(context.Background(), jobFor("sub-1", "user-logged-in"))
	require.NoError(t, err)
	assert.Equal(t, "secret-from-application", creds.SigningSecret)

	// Same when the subscription has no connection at all.
	w = newCredsWorld()
	w.subs["sub-1"].ConnectionID = nil
	creds, err = newDeliveryCredsResolver(w.deps())(context.Background(), jobFor("sub-1", "user-logged-in"))
	require.NoError(t, err)
	assert.Equal(t, "secret-from-application", creds.SigningSecret)
}

// A direct job (no subscription): the code's first segment is the
// application; a bare code resolves nothing, with a reason.
func TestDeliveryCredsDirectJobResolvesThroughItsQualifiedCode(t *testing.T) {
	w := newCredsWorld()
	creds, err := newDeliveryCredsResolver(w.deps())(context.Background(), jobFor("", "value:invoice:created"))
	require.NoError(t, err)
	assert.Equal(t, "secret-from-application", creds.SigningSecret)

	creds, err = newDeliveryCredsResolver(w.deps())(context.Background(), jobFor("", "legacy"))
	require.NoError(t, err)
	assert.True(t, creds.Empty())
	assert.Equal(t, "no subscription, connection or application names a service account", creds.Reason)

	creds, err = newDeliveryCredsResolver(w.deps())(context.Background(), jobFor("", "nosuchapp:x"))
	require.NoError(t, err)
	assert.Equal(t, "application nosuchapp does not exist", creds.Reason)
}

// A lookup error propagates so the caller can WARN and deliver bare; the
// resolver itself never swallows it into "no credentials".
func TestDeliveryCredsLookupErrorPropagates(t *testing.T) {
	w := newCredsWorld()
	d := w.deps()
	d.connection = func(context.Context, string) (*connection.Connection, error) {
		return nil, errors.New("db down")
	}
	_, err := newDeliveryCredsResolver(d)(context.Background(), jobFor("sub-1", "user-logged-in"))
	require.EqualError(t, err, "db down")
}
