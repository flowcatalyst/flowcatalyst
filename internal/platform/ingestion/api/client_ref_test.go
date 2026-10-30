package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/go-sdk/usecase"
)

type fakeClients struct{ byID, byIdentifier map[string]*client.Client }

func (f fakeClients) FindByID(_ context.Context, id string) (*client.Client, error) {
	return f.byID[id], nil
}

func (f fakeClients) FindByIdentifier(_ context.Context, ident string) (*client.Client, error) {
	return f.byIdentifier[ident], nil
}

// A client has one scope however it is named: the id, the identifier, or the
// identifier in another case all resolve to the id; an unknown client is a 404
// rather than a reference to nothing.
func TestResolveClient(t *testing.T) {
	acme := &client.Client{ID: "clt_acme00000000001", Identifier: "acme"}
	s := &State{Clients: fakeClients{
		byID:         map[string]*client.Client{string(acme.ID): acme},
		byIdentifier: map[string]*client.Client{"acme": acme},
	}}
	for _, ref := range []string{"clt_acme00000000001", "acme", "ACME", "  acme "} {
		got, err := s.resolveClient(context.Background(), ref)
		require.NoError(t, err, ref)
		assert.Equal(t, acme.ID, got, ref)
	}

	_, err := s.resolveClient(context.Background(), "nobody")
	require.Error(t, err)
	ue := usecase.AsError(err)
	assert.Equal(t, usecase.KindNotFound, ue.Kind)
	assert.Equal(t, "CLIENT_NOT_FOUND", ue.Code)

	_, err = s.resolveClient(context.Background(), " ")
	require.Error(t, err)
	assert.Equal(t, usecase.KindValidation, usecase.AsError(err).Kind, "a blank reference is a 400, not a 404")
}
