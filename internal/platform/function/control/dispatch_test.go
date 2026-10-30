//go:build integration

package control

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	fncontrol "github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// fixtureConnCode is the connection every fixture's creator declares (a
// declaration must name one): application-scoped, so parallel fixtures never
// share it. newDispatchFixture seeds it ACTIVE.
const fixtureConnCode = "fixture-conn"

// dispatchDescribe is a describe document with a webhook endpoint and the
// given dispatch targets ("app.fn/path" strings).
func dispatchDescribe(t *testing.T, endpointAuth string, dispatches ...string) json.RawMessage {
	t.Helper()
	d := map[string]any{
		"abi":       1,
		"endpoints": []map[string]any{{"method": "POST", "path": "/deliver", "auth": endpointAuth}},
	}
	var ts []map[string]string
	for _, x := range dispatches {
		i := strings.Index(x, "/")
		ts = append(ts, map[string]string{"target": x[:i], "path": x[i:], "connection": fixtureConnCode})
	}
	if len(ts) > 0 {
		d["dispatches"] = ts
	}
	b, err := json.Marshal(d)
	require.NoError(t, err)
	return b
}

type dispatchFixture struct {
	s       *State
	creator *function.Function
	target  *function.Function
	app     string
	// conn is the ACTIVE connection the creator's declarations name.
	conn *connection.Connection
}

// newDispatchFixture seeds an application with a creator (v1 declaring
// <app>.logger/deliver) and a logger target with a live version that has a
// webhook endpoint at /deliver.
func newDispatchFixture(t *testing.T, mutateCreator func(*function.Function), mutateTarget func(*function.Function)) *dispatchFixture {
	t.Helper()
	s := newTestState(t)
	s.Jobs = dispatchjob.NewRepository(testpg.Pool(t))
	s.RunnerURLTemplate = "http://runner-{pool}.test:8095"
	appID := "app_" + shortID(t)
	app := "dsp" + shortID(t)
	seedApplication(t, testpg.Pool(t), appID, app)
	creator := seedFunction(t, testpg.Pool(t), s.Repo, appID, app, "creator", mutateCreator)
	seedVersion(t, s.Repo, creator.ID, 1, function.VersionPublished, "js", dispatchDescribe(t, "platform", app+".logger/deliver"))
	connRepo := connection.NewRepository(testpg.Pool(t))
	s.Connections = connRepo
	s.Pools = dispatchpool.NewRepository(testpg.Pool(t))
	fixtureConn := connection.New(fixtureConnCode, fixtureConnCode, "sa_fixture_"+shortID(t))
	fixtureConn.ApplicationCode = &app
	withTx(t, testpg.Pool(t), func(tx *usecasepgx.DbTx) error {
		return connRepo.Persist(context.Background(), fixtureConn, tx)
	})
	t.Cleanup(func() {
		withTx(t, testpg.Pool(t), func(tx *usecasepgx.DbTx) error {
			return connRepo.Delete(context.Background(), fixtureConn, tx)
		})
	})
	target := seedFunction(t, testpg.Pool(t), s.Repo, appID, app, "logger", mutateTarget)
	tv := seedVersion(t, s.Repo, target.ID, 1, function.VersionPublished, "js", dispatchDescribe(t, "webhook"))
	seedAlias(t, s.Repo, target.ID, function.LiveAlias, tv.ID)
	return &dispatchFixture{s: s, creator: creator, target: target, app: app, conn: fixtureConn}
}

func (f *dispatchFixture) req(d abi.Dispatch, payload string) fncontrol.DispatchRequest {
	if d.Target == "" {
		d.Target = f.app + ".logger"
	}
	if d.Path == "" {
		d.Path = "/deliver"
	}
	return fncontrol.DispatchRequest{FunctionID: f.creator.ID, Version: 1, InvocationID: "inv_7", Dispatch: d, Payload: []byte(payload)}
}

func (f *dispatchFixture) call(ctx context.Context, r fncontrol.DispatchRequest) (*apicommon.Out[fncontrol.DispatchResponse], error) {
	return f.s.dispatch(ctx, &apicommon.In[fncontrol.DispatchRequest]{Body: r})
}

// jobsFrom counts the dispatch jobs recorded as created by the creator.
func (f *dispatchFixture) jobsFrom(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, testpg.Pool(t).QueryRow(context.Background(),
		`SELECT count(*) FROM msg_dispatch_jobs WHERE source = $1`, "function:"+f.creator.Address).Scan(&n))
	return n
}

func TestDispatch_Success(t *testing.T) {
	t.Parallel()
	client := "cli_dsp_" + shortID(t)
	f := newDispatchFixture(t, func(fn *function.Function) { fn.ClientID = &client }, func(fn *function.Function) { fn.ClientID = &client })

	r := f.req(abi.Dispatch{
		Subject: "order/42", MessageGroup: "g1", Mode: "IMMEDIATE",
		Metadata: []abi.KV{{Key: "tenant", Value: "acme"}},
	}, `{"order":42}`)
	r.RequestCorrelationID = "corr-from-request"
	out, err := f.call(anchorCtx(), r)
	require.NoError(t, err)
	require.NotEmpty(t, out.Body.JobID)

	job, err := f.s.Jobs.FindByID(context.Background(), out.Body.JobID)
	require.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, dispatchjob.KindTask, job.Kind)
	require.NotNil(t, job.Source)
	assert.Equal(t, "function:"+f.creator.Address, *job.Source)
	assert.Equal(t, f.app+":function:creator:dispatch", job.Code, "default code is owned by the function's application")
	assert.Equal(t, "http://runner-default.test:8095/fn/"+f.app+".logger/deliver", job.TargetURL)
	require.NotNil(t, job.Payload)
	assert.JSONEq(t, `{"order":42}`, *job.Payload)
	assert.Equal(t, "application/json", job.PayloadContentType)
	assert.True(t, job.DataOnly, "a function target gets the data, not an envelope")
	require.NotNil(t, job.ClientID)
	assert.Equal(t, client, *job.ClientID, "the job belongs to the function's client")
	require.NotNil(t, job.CorrelationID)
	assert.Equal(t, "corr-from-request", *job.CorrelationID)
	require.NotNil(t, job.Subject)
	assert.Equal(t, "order/42", *job.Subject)
	require.NotNil(t, job.MessageGroup)
	assert.Equal(t, "g1", *job.MessageGroup)
	assert.Equal(t, "IMMEDIATE", string(job.Mode))
	assert.Equal(t, "PENDING", string(job.Status))
	assert.Nil(t, job.ServiceAccountID, "no credential travels with the job; the connection's account signs")
	require.NotNil(t, job.ConnectionID, "the job carries its connection")
	assert.Equal(t, f.conn.ID, *job.ConnectionID)
	assert.Equal(t, []dispatchjob.Metadata{
		{Key: "tenant", Value: "acme"},
		{Key: "fn.address", Value: f.creator.Address},
		{Key: "fn.version", Value: "1"},
		{Key: "fn.invocation", Value: "inv_7"},
	}, job.Metadata, "metadata is an array of {key,value}; provenance is the platform's")
}

func TestDispatch_GuestCorrelationAndCausationWin(t *testing.T) {
	t.Parallel()
	f := newDispatchFixture(t, nil, nil)
	r := f.req(abi.Dispatch{CorrelationID: "mine", CausationID: "cause-1"}, `{}`)
	r.RequestCorrelationID = "from-request"
	out, err := f.call(anchorCtx(), r)
	require.NoError(t, err)
	job, err := f.s.Jobs.FindByID(context.Background(), out.Body.JobID)
	require.NoError(t, err)
	require.NotNil(t, job.CorrelationID)
	assert.Equal(t, "mine", *job.CorrelationID)
	assert.Contains(t, job.Metadata, dispatchjob.Metadata{Key: "causationId", Value: "cause-1"})
}

func TestDispatch_ExplicitZeroRetriesIsHonoured(t *testing.T) {
	t.Parallel()
	f := newDispatchFixture(t, nil, nil)
	zero := 0
	out, err := f.call(anchorCtx(), f.req(abi.Dispatch{MaxRetries: &zero}, `{}`))
	require.NoError(t, err)
	job, err := f.s.Jobs.FindByID(context.Background(), out.Body.JobID)
	require.NoError(t, err)
	assert.Equal(t, uint32(0), job.MaxRetries)

	out, err = f.call(anchorCtx(), f.req(abi.Dispatch{}, `{}`))
	require.NoError(t, err)
	job, err = f.s.Jobs.FindByID(context.Background(), out.Body.JobID)
	require.NoError(t, err)
	assert.Equal(t, uint32(3), job.MaxRetries, "absent keeps the platform default")
}

func TestDispatch_NotDeclared(t *testing.T) {
	t.Parallel()
	f := newDispatchFixture(t, nil, nil)
	for name, d := range map[string]abi.Dispatch{
		"other target": {Target: f.app + ".other", Path: "/deliver"},
		"other path":   {Target: f.app + ".logger", Path: "/other"},
		"url":          {Target: "https://evil.example", Path: "/deliver"},
	} {
		_, err := f.call(anchorCtx(), f.req(d, `{}`))
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "DISPATCH_NOT_DECLARED")
		_ = name
	}
	assert.Zero(t, f.jobsFrom(t), "a refusal writes nothing")
}

// Describe is per version: a declaration on v2 does not authorise a call the
// runner made as v1.
func TestDispatch_DeclaredOnlyByAnotherVersion(t *testing.T) {
	t.Parallel()
	f := newDispatchFixture(t, nil, nil)
	seedVersion(t, f.s.Repo, f.creator.ID, 2, function.VersionPublished, "js", dispatchDescribe(t, "platform", f.app+".logger/deliver", f.app+".logger/extra"))
	r := f.req(abi.Dispatch{Path: "/extra"}, `{}`)
	r.Version = 1
	_, err := f.call(anchorCtx(), r)
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "DISPATCH_NOT_DECLARED")
	assert.Zero(t, f.jobsFrom(t))
}

func TestDispatch_TargetOfAnotherApplicationIsRefused(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	s.Jobs = dispatchjob.NewRepository(testpg.Pool(t))
	appID, app := "app_"+shortID(t), "dspa"+shortID(t)
	otherID, other := "app_o"+shortID(t), "dspo"+shortID(t)
	seedApplication(t, testpg.Pool(t), appID, app)
	seedApplication(t, testpg.Pool(t), otherID, other)
	// The creator DECLARES a function of the other application (publish
	// refuses this; the control plane must not rely on that alone).
	creator := seedFunction(t, testpg.Pool(t), s.Repo, appID, app, "creator", nil)
	seedVersion(t, s.Repo, creator.ID, 1, function.VersionPublished, "js", dispatchDescribe(t, "platform", other+".logger/deliver", app+".sneaky/deliver"))
	foreign := seedFunction(t, testpg.Pool(t), s.Repo, otherID, other, "logger", nil)
	fv := seedVersion(t, s.Repo, foreign.ID, 1, function.VersionPublished, "js", dispatchDescribe(t, "webhook"))
	seedAlias(t, s.Repo, foreign.ID, function.LiveAlias, fv.ID)

	_, err := s.dispatch(anchorCtx(), &apicommon.In[fncontrol.DispatchRequest]{Body: fncontrol.DispatchRequest{
		FunctionID: creator.ID, Version: 1, Dispatch: abi.Dispatch{Target: other + ".logger", Path: "/deliver"}, Payload: []byte(`{}`),
	}})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "DISPATCH_TARGET_NOT_OWNED")

	// An address that CLAIMS the creator's application but whose function row
	// belongs to another one (a code rename, or a forged row) is refused too.
	sneaky := seedFunction(t, testpg.Pool(t), s.Repo, otherID, app, "sneaky", nil)
	sv := seedVersion(t, s.Repo, sneaky.ID, 1, function.VersionPublished, "js", dispatchDescribe(t, "webhook"))
	seedAlias(t, s.Repo, sneaky.ID, function.LiveAlias, sv.ID)
	_, err = s.dispatch(anchorCtx(), &apicommon.In[fncontrol.DispatchRequest]{Body: fncontrol.DispatchRequest{
		FunctionID: creator.ID, Version: 1, Dispatch: abi.Dispatch{Target: app + ".sneaky", Path: "/deliver"}, Payload: []byte(`{}`),
	}})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "DISPATCH_TARGET_NOT_OWNED")
}

// A foreign address is refused as NOT OWNED even when no such function exists,
// so the answer does not reveal whether another application has a function by
// that name.
func TestDispatch_ForeignTargetDoesNotLeakExistence(t *testing.T) {
	t.Parallel()
	f := newDispatchFixture(t, nil, nil)
	seedVersion(t, f.s.Repo, f.creator.ID, 2, function.VersionPublished, "js", dispatchDescribe(t, "platform", "nobodys.ghost/deliver"))
	r := f.req(abi.Dispatch{Target: "nobodys.ghost"}, `{}`)
	r.Version = 2
	_, err := f.call(anchorCtx(), r)
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "DISPATCH_TARGET_NOT_OWNED")
}

func TestDispatch_TargetOfAnotherClientIsRefused(t *testing.T) {
	t.Parallel()
	a, b := "cli_a_"+shortID(t), "cli_b_"+shortID(t)
	t.Run("client to other client", func(t *testing.T) {
		f := newDispatchFixture(t, func(fn *function.Function) { fn.ClientID = &a }, func(fn *function.Function) { fn.ClientID = &b })
		_, err := f.call(anchorCtx(), f.req(abi.Dispatch{}, `{}`))
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "DISPATCH_TARGET_CLIENT_MISMATCH")
		assert.Zero(t, f.jobsFrom(t))
	})
	t.Run("platform to client", func(t *testing.T) {
		f := newDispatchFixture(t, nil, func(fn *function.Function) { fn.ClientID = &b })
		_, err := f.call(anchorCtx(), f.req(abi.Dispatch{}, `{}`))
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "DISPATCH_TARGET_CLIENT_MISMATCH")
		assert.Zero(t, f.jobsFrom(t))
	})
}

func TestDispatch_TargetMissing(t *testing.T) {
	t.Parallel()
	f := newDispatchFixture(t, nil, nil)
	seedVersion(t, f.s.Repo, f.creator.ID, 2, function.VersionPublished, "js", dispatchDescribe(t, "platform", f.app+".ghost/deliver"))
	r := f.req(abi.Dispatch{Target: f.app + ".ghost"}, `{}`)
	r.Version = 2
	_, err := f.call(anchorCtx(), r)
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "DISPATCH_TARGET_NOT_FOUND")
}

// A job must never be created for a URL that cannot verify the delivery.
func TestDispatch_TargetCannotTakeASignedPost(t *testing.T) {
	t.Parallel()
	f := newDispatchFixture(t, nil, nil)

	// no live alias
	noLive := seedFunction(t, testpg.Pool(t), f.s.Repo, f.creator.ApplicationID, f.app, "nolive", nil)
	seedVersion(t, f.s.Repo, noLive.ID, 1, function.VersionPublished, "js", dispatchDescribe(t, "webhook"))
	// live, but the path is a platform-auth endpoint
	plat := seedFunction(t, testpg.Pool(t), f.s.Repo, f.creator.ApplicationID, f.app, "plat", nil)
	pv := seedVersion(t, f.s.Repo, plat.ID, 1, function.VersionPublished, "js", dispatchDescribe(t, "platform"))
	seedAlias(t, f.s.Repo, plat.ID, function.LiveAlias, pv.ID)
	// live, but nothing at the path
	bare := seedFunction(t, testpg.Pool(t), f.s.Repo, f.creator.ApplicationID, f.app, "bare", nil)
	bv := seedVersion(t, f.s.Repo, bare.ID, 1, function.VersionPublished, "js", dispatchDescribe(t, "webhook"))
	seedAlias(t, f.s.Repo, bare.ID, function.LiveAlias, bv.ID)

	seedVersion(t, f.s.Repo, f.creator.ID, 2, function.VersionPublished, "js",
		dispatchDescribe(t, "platform", f.app+".nolive/deliver", f.app+".plat/deliver", f.app+".bare/missing"))
	for _, d := range []abi.Dispatch{
		{Target: f.app + ".nolive", Path: "/deliver"},
		{Target: f.app + ".plat", Path: "/deliver"},
		{Target: f.app + ".bare", Path: "/missing"},
	} {
		r := f.req(d, `{}`)
		r.Version = 2
		_, err := f.call(anchorCtx(), r)
		testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, "DISPATCH_TARGET_UNAVAILABLE")
	}
	assert.Zero(t, f.jobsFrom(t))
}

func TestDispatch_FieldValidation(t *testing.T) {
	t.Parallel()
	f := newDispatchFixture(t, nil, nil)
	neg, zero := -1, 0
	for name, c := range map[string]struct {
		d       abi.Dispatch
		payload string
		code    string
	}{
		"foreign code":      {abi.Dispatch{Code: "other:x:y:z"}, `{}`, "DISPATCH_CODE_NOT_OWNED"},
		"code whitespace":   {abi.Dispatch{Code: f.app + ":x y"}, `{}`, "DISPATCH_CODE_NOT_OWNED"},
		"bad mode":          {abi.Dispatch{Mode: "FAST"}, `{}`, "DISPATCH_MODE_INVALID"},
		"negative retries":  {abi.Dispatch{MaxRetries: &neg}, `{}`, "DISPATCH_MAX_RETRIES_INVALID"},
		"zero timeout":      {abi.Dispatch{TimeoutSeconds: &zero}, `{}`, "DISPATCH_TIMEOUT_INVALID"},
		"not json":          {abi.Dispatch{}, `not json`, "DISPATCH_PAYLOAD_NOT_JSON"},
		"not utf8":          {abi.Dispatch{ContentType: "application/octet-stream"}, "\xff\xfe", "DISPATCH_PAYLOAD_NOT_TEXT"},
		"too large":         {abi.Dispatch{ContentType: "text/plain"}, strings.Repeat("x", maxDispatchPayloadBytes+1), "DISPATCH_PAYLOAD_TOO_LARGE"},
		"reserved metadata": {abi.Dispatch{Metadata: []abi.KV{{Key: "fn.address", Value: "forged"}}}, `{}`, "DISPATCH_METADATA_RESERVED"},
		"reserved upper":    {abi.Dispatch{Metadata: []abi.KV{{Key: "FN.x", Value: "forged"}}}, `{}`, "DISPATCH_METADATA_RESERVED"},
		"empty key":         {abi.Dispatch{Metadata: []abi.KV{{Key: " ", Value: "v"}}}, `{}`, "DISPATCH_METADATA_INVALID"},
		"duplicate key":     {abi.Dispatch{Metadata: []abi.KV{{Key: "a", Value: "1"}, {Key: "a", Value: "2"}}}, `{}`, "DISPATCH_METADATA_INVALID"},
		"too many":          {abi.Dispatch{Metadata: manyKV(maxDispatchMetadata + 1)}, `{}`, "DISPATCH_METADATA_TOO_MANY"},
		"long correlation":  {abi.Dispatch{CorrelationID: strings.Repeat("c", 101)}, `{}`, "DISPATCH_CORRELATION_TOO_LONG"},
	} {
		_, err := f.call(anchorCtx(), f.req(c.d, c.payload))
		ue := usecase.AsError(err)
		if assert.NotNil(t, ue, name) {
			assert.Equal(t, c.code, ue.Code, name)
			assert.Equal(t, usecase.KindValidation, ue.Kind, name)
		}
	}
	assert.Zero(t, f.jobsFrom(t))
}

func manyKV(n int) []abi.KV {
	out := make([]abi.KV, n)
	for i := range out {
		out[i] = abi.KV{Key: "k" + itoaSeq(i), Value: "v"}
	}
	return out
}

func TestDispatch_RequiresTheRunnerRole(t *testing.T) {
	t.Parallel()
	f := newDispatchFixture(t, nil, nil)
	_, err := f.call(anchorCtx("platform:function:*:view"), f.req(abi.Dispatch{}, `{}`))
	require.Error(t, err, "an anchor without platform:function:runner:control must be refused")
	_, err = f.call(context.Background(), f.req(abi.Dispatch{}, `{}`))
	require.Error(t, err, "no principal at all must be refused")
	// The role alone is not enough: the control plane is anchor-scoped.
	clientCtx := auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "sa_client_scoped", Scope: auth.ScopeClient, Permissions: []string{"platform:function:runner:control"},
	})
	_, err = f.call(clientCtx, f.req(abi.Dispatch{}, `{}`))
	require.Error(t, err, "a non-anchor holding the runner permission must be refused")
	assert.Zero(t, f.jobsFrom(t))
}

func TestDispatch_UnknownFunctionAndVersion(t *testing.T) {
	t.Parallel()
	f := newDispatchFixture(t, nil, nil)
	r := f.req(abi.Dispatch{}, `{}`)
	r.FunctionID = "fnc_missing"
	_, err := f.call(anchorCtx(), r)
	require.Error(t, err)
	r = f.req(abi.Dispatch{}, `{}`)
	r.Version = 9
	_, err = f.call(anchorCtx(), r)
	require.Error(t, err)
	_, err = f.call(anchorCtx(), fncontrol.DispatchRequest{FunctionID: f.creator.ID, Version: 1})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "DISPATCH_TARGET_REQUIRED")
}

// The runner's own client against the real route behind the real
// authenticator: the payload survives base64, a created job is a 201, and each
// refusal reaches the guest as the code its class maps to.
func TestDispatch_ClientAgainstRealServer(t *testing.T) {
	t.Parallel()
	f := newDispatchFixture(t, nil, nil)
	ti := newTestTokenIssuer(t)
	srv := newTestServer(t, f.s, ti)
	runner := ti.mint(t, "sa_fn_runner_dsp", []string{"platform:function:runner:control"})
	client := fncontrol.NewClient(srv.URL, func(context.Context) (string, error) { return runner, nil }, nil)

	ctx := context.Background()
	out, aerr := client.Dispatch(ctx, f.req(abi.Dispatch{Metadata: []abi.KV{{Key: "k", Value: "v"}}}, `{"é":"✓"}`))
	require.Nil(t, aerr)
	job, err := f.s.Jobs.FindByID(ctx, out.JobID)
	require.NoError(t, err)
	require.NotNil(t, job)
	assert.JSONEq(t, `{"é":"✓"}`, *job.Payload)

	// 403 -> NOT_ALLOWED (not declared); 400 -> BAD_REQUEST (validation);
	// a missing target is a BAD_REQUEST too (never retried).
	_, aerr = client.Dispatch(ctx, f.req(abi.Dispatch{Path: "/other"}, `{}`))
	require.NotNil(t, aerr)
	assert.Equal(t, abi.CodeNotAllowed, aerr.Code)
	_, aerr = client.Dispatch(ctx, f.req(abi.Dispatch{Mode: "FAST"}, `{}`))
	require.NotNil(t, aerr)
	assert.Equal(t, abi.CodeBadRequest, aerr.Code)

	// A token without the runner permission is refused outright.
	weak := ti.mint(t, "sa_not_a_runner", []string{"platform:function:*:view"})
	client = fncontrol.NewClient(srv.URL, func(context.Context) (string, error) { return weak, nil }, nil)
	_, aerr = client.Dispatch(ctx, f.req(abi.Dispatch{}, `{}`))
	require.NotNil(t, aerr)
	assert.Equal(t, abi.CodeNotAllowed, aerr.Code)
}
