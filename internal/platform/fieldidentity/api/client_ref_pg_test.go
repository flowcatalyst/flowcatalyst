//go:build integration

package api

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// identifier loads the fixture client's identifier slug.
func (f *fixture) identifier() string {
	f.t.Helper()
	c, err := f.s.Ops.Clients.FindByID(context.Background(), f.clientID)
	require.NoError(f.t, err)
	require.NotNil(f.t, c)
	return c.Identifier
}

func TestClientRefIdentifierAccepted(t *testing.T) {
	f := newFixture(t, hasher(t))
	ident := f.identifier()

	// create a worker through the identifier: stored and returned as the id.
	out, err := f.s.ensureWorker(f.actx, &apicommon.In[EnsureFieldWorkerRequest]{Body: EnsureFieldWorkerRequest{
		ClientID: strings.ToUpper(ident), WorkerCode: "W-REF", Name: "Ref", AppCodes: []string{f.codeA},
	}})
	require.NoError(t, err)
	assert.Equal(t, f.clientID, out.Body.ClientID)

	got, err := f.s.getWorker(f.actx, &apicommon.IDInput{ID: out.Body.ID})
	require.NoError(t, err)
	assert.Equal(t, f.clientID, got.Body.ClientID)

	list, err := f.s.listWorkers(f.actx, &listWorkersInput{ClientID: ident})
	require.NoError(t, err)
	assert.EqualValues(t, 1, list.Body.Total)

	// devices: enrolment code, list.
	code, err := f.s.createCode(f.actx, &apicommon.In[CreateEnrolmentCodeRequest]{Body: CreateEnrolmentCodeRequest{
		ClientID: ident, Mode: "SHARED", AppCodes: []string{f.codeA},
	}})
	require.NoError(t, err)
	assert.NotEmpty(t, code.Body.Code)
	f.device("SHARED", nil, f.appA)
	devs, err := f.s.listDevices(f.actx, &listDevicesInput{ClientID: ident})
	require.NoError(t, err)
	assert.EqualValues(t, 1, devs.Body.Total)
	assert.Equal(t, f.clientID, devs.Body.Devices[0].ClientID)

	// policy by path.
	put, err := f.s.putPolicy(f.actx, &putPolicyInput{ClientID: ident, Body: PutFieldPolicyRequest{}})
	require.NoError(t, err)
	assert.Equal(t, f.clientID, put.Body.ClientID)
	pol, err := f.s.getPolicy(f.actx, &policyInput{ClientID: ident})
	require.NoError(t, err)
	assert.Equal(t, f.clientID, pol.Body.ClientID)
}

func TestClientRefUnknownSameError(t *testing.T) {
	f := newFixture(t, hasher(t))
	_, errID := f.s.listWorkers(f.actx, &listWorkersInput{ClientID: "clt_doesnotexist0"})
	_, errSlug := f.s.listWorkers(f.actx, &listWorkersInput{ClientID: "no-such-client"})
	requireErr(t, errID, usecase.KindNotFound, "Client_NOT_FOUND")
	requireErr(t, errSlug, usecase.KindNotFound, "Client_NOT_FOUND")

	_, err := f.s.listDevices(f.actx, &listDevicesInput{ClientID: "no-such-client"})
	requireErr(t, err, usecase.KindNotFound, "Client_NOT_FOUND")
	_, err = f.s.getPolicy(f.actx, &policyInput{ClientID: "no-such-client"})
	requireErr(t, err, usecase.KindNotFound, "Client_NOT_FOUND")
}
