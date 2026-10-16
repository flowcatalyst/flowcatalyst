//go:build integration

package control

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	fncontrol "github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// poolDescribe is the creator's describe declaring <app>.logger/deliver with
// the given dispatch pool and connection codes ("" declares none).
func poolDescribe(t *testing.T, app, pool, conn string) json.RawMessage {
	t.Helper()
	entry := map[string]string{"target": app + ".logger", "path": "/deliver"}
	if pool != "" {
		entry["pool"] = pool
	}
	if conn != "" {
		entry["connection"] = conn
	}
	b, err := json.Marshal(map[string]any{
		"abi":        1,
		"endpoints":  []map[string]any{{"method": "POST", "path": "/deliver", "auth": "platform"}},
		"dispatches": []map[string]string{entry},
	})
	require.NoError(t, err)
	return b
}

// poolFixture is a dispatch fixture whose creator's version 2 declares a
// dispatch pool.
type poolFixture struct {
	*dispatchFixture
	appID string
}

func newPoolFixture(t *testing.T, declared string, mutateCreator func(*function.Function), mutateTarget func(*function.Function)) *poolFixture {
	t.Helper()
	return newDeclFixture(t, declared, "", mutateCreator, mutateTarget)
}

// newDeclFixture declares a pool and/or a connection ("" = none).
func newDeclFixture(t *testing.T, pool, conn string, mutateCreator func(*function.Function), mutateTarget func(*function.Function)) *poolFixture {
	t.Helper()
	f := newDispatchFixture(t, mutateCreator, mutateTarget)
	seedVersion(t, f.s.Repo, f.creator.ID, 2, function.VersionPublished, "js", poolDescribe(t, f.app, pool, conn))
	f.s.Pools = dispatchpool.NewRepository(testpg.Pool(t))
	f.s.Connections = connection.NewRepository(testpg.Pool(t))
	return &poolFixture{dispatchFixture: f, appID: f.creator.ApplicationID}
}

// req is a version-2 request (the one declaring the pool).
func (f *poolFixture) req(d abi.Dispatch) fncontrol.DispatchRequest {
	r := f.dispatchFixture.req(d, `{}`)
	r.Version = 2
	return r
}

// seedPool persists an ACTIVE pool; mutate may change it first.
func seedPool(t *testing.T, f *poolFixture, code string, mutate func(*dispatchpool.DispatchPool)) *dispatchpool.DispatchPool {
	t.Helper()
	p := dispatchpool.New(code, code)
	if mutate != nil {
		mutate(p)
	}
	pool := testpg.Pool(t)
	withTx(t, pool, func(tx *usecasepgx.DbTx) error { return f.s.Pools.Persist(context.Background(), p, tx) })
	t.Cleanup(func() {
		withTx(t, pool, func(tx *usecasepgx.DbTx) error { return f.s.Pools.Delete(context.Background(), p, tx) })
	})
	return p
}

func poolCode(t *testing.T, name string) string {
	t.Helper()
	return strings.ToLower("dp-" + name + "-" + shortID(t))
}

func (f *poolFixture) jobPool(t *testing.T, jobID string) *string {
	t.Helper()
	job, err := f.s.Jobs.FindByID(context.Background(), jobID)
	require.NoError(t, err)
	require.NotNil(t, job)
	return job.DispatchPoolID
}

func TestDispatchPool_DeclaredPoolIsSetOnTheJob(t *testing.T) {
	t.Parallel()
	code := poolCode(t, "ok")
	f := newPoolFixture(t, code, nil, nil)
	p := seedPool(t, f, code, nil)

	out, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
	require.NoError(t, err)
	got := f.jobPool(t, out.Body.JobID)
	require.NotNil(t, got, "the job must carry the declared pool")
	assert.Equal(t, p.ID, *got)
}

func TestDispatchPool_NoDeclaredPoolIsUnchanged(t *testing.T) {
	t.Parallel()
	f := newPoolFixture(t, "", nil, nil)
	// A pool exists and is even visible; with none declared it is not used.
	seedPool(t, f, poolCode(t, "unused"), nil)
	out, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
	require.NoError(t, err)
	assert.Nil(t, f.jobPool(t, out.Body.JobID), "no declared pool means the client default (no pool id)")
}

func TestDispatchPool_UnknownPool(t *testing.T) {
	t.Parallel()
	f := newPoolFixture(t, poolCode(t, "ghost"), nil, nil)
	_, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "DISPATCH_POOL_NOT_FOUND")
	assert.Zero(t, f.jobsFrom(t))
}

func TestDispatchPool_NotActiveIsRefused(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*dispatchpool.DispatchPool){
		"suspended": func(p *dispatchpool.DispatchPool) { p.Suspend() },
		"archived":  func(p *dispatchpool.DispatchPool) { p.Archive() },
	} {
		t.Run(name, func(t *testing.T) {
			code := poolCode(t, name)
			f := newPoolFixture(t, code, nil, nil)
			seedPool(t, f, code, mutate)
			_, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
			testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "DISPATCH_POOL_NOT_ALLOWED")
			assert.Zero(t, f.jobsFrom(t))
		})
	}
}

func TestDispatchPool_ClientRules(t *testing.T) {
	t.Parallel()
	a, b := "cli_a_"+shortID(t), "cli_b_"+shortID(t)

	t.Run("a client function may use a client-less pool", func(t *testing.T) {
		code := poolCode(t, "shared")
		f := newPoolFixture(t, code,
			func(fn *function.Function) { fn.ClientID = &a }, func(fn *function.Function) { fn.ClientID = &a })
		p := seedPool(t, f, code, nil)
		out, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
		require.NoError(t, err)
		require.NotNil(t, f.jobPool(t, out.Body.JobID))
		assert.Equal(t, p.ID, *f.jobPool(t, out.Body.JobID))
	})
	t.Run("a client function may use its own client's pool, which wins over a client-less one", func(t *testing.T) {
		code := poolCode(t, "own")
		f := newPoolFixture(t, code,
			func(fn *function.Function) { fn.ClientID = &a }, func(fn *function.Function) { fn.ClientID = &a })
		seedPool(t, f, code, nil) // client-less, same code
		own := seedPool(t, f, code, func(p *dispatchpool.DispatchPool) { p.ClientID = ids.PtrOf[ids.ClientID](&a) })
		out, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
		require.NoError(t, err)
		require.NotNil(t, f.jobPool(t, out.Body.JobID))
		assert.Equal(t, own.ID, *f.jobPool(t, out.Body.JobID))
	})
	t.Run("another client's pool is refused", func(t *testing.T) {
		code := poolCode(t, "other")
		f := newPoolFixture(t, code,
			func(fn *function.Function) { fn.ClientID = &a }, func(fn *function.Function) { fn.ClientID = &a })
		seedPool(t, f, code, func(p *dispatchpool.DispatchPool) { p.ClientID = ids.PtrOf[ids.ClientID](&b) })
		_, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
		// Not visible at all: the same answer as a pool that does not exist,
		// so another tenant's pool codes are not enumerable.
		testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "DISPATCH_POOL_NOT_FOUND")
		assert.Zero(t, f.jobsFrom(t))
	})
	t.Run("a client-less function cannot use a client's pool", func(t *testing.T) {
		code := poolCode(t, "clientonly")
		f := newPoolFixture(t, code, nil, nil)
		seedPool(t, f, code, func(p *dispatchpool.DispatchPool) { p.ClientID = ids.PtrOf[ids.ClientID](&b) })
		_, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
		testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "DISPATCH_POOL_NOT_FOUND")
		assert.Zero(t, f.jobsFrom(t))
	})
	t.Run("a suspended own-client pool is refused, not skipped for the client-less one", func(t *testing.T) {
		code := poolCode(t, "ownsusp")
		f := newPoolFixture(t, code,
			func(fn *function.Function) { fn.ClientID = &a }, func(fn *function.Function) { fn.ClientID = &a })
		seedPool(t, f, code, nil)
		seedPool(t, f, code, func(p *dispatchpool.DispatchPool) {
			p.ClientID = ids.PtrOf[ids.ClientID](&a)
			p.Suspend()
		})
		_, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "DISPATCH_POOL_NOT_ALLOWED")
		assert.Zero(t, f.jobsFrom(t))
	})
}

func TestDispatchPool_OwnershipRules(t *testing.T) {
	t.Parallel()
	t.Run("a pool owned by the function's application is allowed", func(t *testing.T) {
		code := poolCode(t, "mine")
		f := newPoolFixture(t, code, nil, nil)
		p := seedPool(t, f, code, func(p *dispatchpool.DispatchPool) { p.ApplicationID = &f.appID })
		out, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
		require.NoError(t, err)
		require.NotNil(t, f.jobPool(t, out.Body.JobID))
		assert.Equal(t, p.ID, *f.jobPool(t, out.Body.JobID))
	})
	t.Run("an unowned pool is allowed", func(t *testing.T) {
		code := poolCode(t, "unowned")
		f := newPoolFixture(t, code, nil, nil)
		seedPool(t, f, code, nil)
		_, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
		require.NoError(t, err)
	})
	t.Run("a pool owned by another application is refused", func(t *testing.T) {
		code := poolCode(t, "theirs")
		f := newPoolFixture(t, code, nil, nil)
		other := "app_other_" + shortID(t)
		seedApplication(t, testpg.Pool(t), other, "othr"+shortID(t))
		seedPool(t, f, code, func(p *dispatchpool.DispatchPool) { p.ApplicationID = &other })
		_, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "DISPATCH_POOL_NOT_ALLOWED")
		assert.Zero(t, f.jobsFrom(t))
	})
}

// The pool is the declaration's. The call names a (target, path) and nothing
// else; the version the runner invoked decides the pool, and a version that
// declares none dispatches to the default even when another version of the
// same function declares one.
func TestDispatchPool_OnlyTheInvokedVersionsDeclarationDecides(t *testing.T) {
	t.Parallel()
	code := poolCode(t, "declv2")
	f := newPoolFixture(t, code, nil, nil)
	seedPool(t, f, code, nil)

	// Version 1 (no pool) is unchanged although version 2 declares one.
	r := f.dispatchFixture.req(abi.Dispatch{}, `{}`)
	out, err := f.call(anchorCtx(), r)
	require.NoError(t, err)
	assert.Nil(t, f.jobPool(t, out.Body.JobID))

	// Version 2 picks the pool.
	out, err = f.call(anchorCtx(), f.req(abi.Dispatch{}))
	require.NoError(t, err)
	assert.NotNil(t, f.jobPool(t, out.Body.JobID))
}

// With several declarations the pool and connection are the MATCHING entry's,
// not another entry's (here the first entry is a different path with other
// values).
func TestDispatchPool_TheMatchingDeclarationDecides(t *testing.T) {
	t.Parallel()
	right, wrong := poolCode(t, "right"), poolCode(t, "wrong")
	rc, wc := connCode(t, "right"), connCode(t, "wrong")
	f := newDeclFixture(t, "", "", nil, nil)
	b, err := json.Marshal(map[string]any{
		"abi":       1,
		"endpoints": []map[string]any{{"method": "POST", "path": "/deliver", "auth": "platform"}},
		"dispatches": []map[string]string{
			{"target": f.app + ".logger", "path": "/zzz", "pool": wrong, "connection": wc},
			{"target": f.app + ".logger", "path": "/deliver", "pool": right, "connection": rc},
		},
	})
	require.NoError(t, err)
	seedVersion(t, f.s.Repo, f.creator.ID, 3, function.VersionPublished, "js", b)
	p := seedPool(t, f, right, nil)
	seedPool(t, f, wrong, nil)
	seedConn(t, f, rc, "sa_right_"+shortID(t), nil)
	seedConn(t, f, wc, "sa_wrong_"+shortID(t), nil)

	r := f.req(abi.Dispatch{})
	r.Version = 3
	out, err := f.call(anchorCtx(), r)
	require.NoError(t, err)
	require.NotNil(t, f.jobPool(t, out.Body.JobID))
	assert.Equal(t, p.ID, *f.jobPool(t, out.Body.JobID))
	require.NotNil(t, f.jobSA(t, out.Body.JobID))
	assert.Equal(t, "sa_right_"+shortID(t), *f.jobSA(t, out.Body.JobID))
}

// Through the real route a guest-supplied `pool` in the dispatch body can
// neither pick a pool nor change the declared one.
func TestDispatchPool_RequestCannotOverrideTheDeclaredPool(t *testing.T) {
	t.Parallel()
	declared, other := poolCode(t, "declared"), poolCode(t, "wanted")
	f := newPoolFixture(t, declared, nil, nil)
	want := seedPool(t, f, declared, nil)
	seedPool(t, f, other, nil)

	ti := newTestTokenIssuer(t)
	srv := newTestServer(t, f.s, ti)
	runner := ti.mint(t, "sa_fn_runner_dspool", []string{"platform:function:runner:control"})
	raw, err := json.Marshal(map[string]any{
		"functionId": f.creator.ID, "version": 2,
		"dispatch": map[string]any{"target": f.app + ".logger", "path": "/deliver", "pool": other},
		"payload":  "e30=",
	})
	require.NoError(t, err)
	resp := postJSON(t, srv.URL+fncontrol.PathDispatchJobs, runner, raw)
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var out fncontrol.DispatchResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
		got := f.jobPool(t, out.JobID)
		require.NotNil(t, got)
		assert.Equal(t, want.ID, *got, "the declared pool, never the one the request named")
	} else {
		assert.Zero(t, f.jobsFrom(t), "a refused request created no job")
	}
}

func postJSON(t *testing.T, url, token string, body []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// Every refusal reaches the guest as the class it maps to, like the other
// dispatch refusals: 403 is NOT_ALLOWED, a missing pool is a 404 and so
// BAD_REQUEST (never retried), and the message names the pool.
func TestDispatchPool_RefusalsReachTheGuest(t *testing.T) {
	t.Parallel()
	ti := newTestTokenIssuer(t)
	runner := ti.mint(t, "sa_fn_runner_dspool2", []string{"platform:function:runner:control"})
	for name, c := range map[string]struct {
		suspend bool
		want    string
	}{"suspended": {true, abi.CodeNotAllowed}, "missing": {false, abi.CodeBadRequest}} {
		t.Run(name, func(t *testing.T) {
			code := poolCode(t, "wire")
			f := newPoolFixture(t, code, nil, nil)
			if c.suspend {
				seedPool(t, f, code, func(p *dispatchpool.DispatchPool) { p.Suspend() })
			}
			srv := newTestServer(t, f.s, ti)
			client := fncontrol.NewClient(srv.URL, func(context.Context) (string, error) { return runner, nil }, nil)
			_, aerr := client.Dispatch(context.Background(), f.req(abi.Dispatch{}))
			require.NotNil(t, aerr)
			assert.Equal(t, c.want, aerr.Code)
			assert.Contains(t, aerr.Message, code, "the guest is told which pool was refused")
		})
	}
}

func TestDispatchPool_UnwiredRepositoryIsAnInternalError(t *testing.T) {
	t.Parallel()
	code := poolCode(t, "unwired")
	f := newPoolFixture(t, code, nil, nil)
	f.s.Pools = nil
	_, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
	testpg.RequireUsecaseError(t, err, usecase.KindInternal, "DISPATCH_UNWIRED")
}

// ---- connections ----

func seedConn(t *testing.T, f *poolFixture, code, saID string, mutate func(*connection.Connection)) *connection.Connection {
	t.Helper()
	c := connection.New(code, code, saID)
	if mutate != nil {
		mutate(c)
	}
	pool := testpg.Pool(t)
	withTx(t, pool, func(tx *usecasepgx.DbTx) error { return f.s.Connections.Persist(context.Background(), c, tx) })
	t.Cleanup(func() {
		withTx(t, pool, func(tx *usecasepgx.DbTx) error { return f.s.Connections.Delete(context.Background(), c, tx) })
	})
	return c
}

func connCode(t *testing.T, name string) string {
	t.Helper()
	return strings.ToLower("cn-" + name + "-" + shortID(t))
}

func (f *poolFixture) jobSA(t *testing.T, jobID string) *string {
	t.Helper()
	job, err := f.s.Jobs.FindByID(context.Background(), jobID)
	require.NoError(t, err)
	require.NotNil(t, job)
	return job.ServiceAccountID
}

// The connection is what names the signing account for a job with no
// subscription: the job carries the connection's service account (the
// delivery resolver honours it), and fn.connection records which connection.
func TestDispatchConnection_DeclaredConnectionSignsTheJob(t *testing.T) {
	t.Parallel()
	code := connCode(t, "ok")
	f := newDeclFixture(t, "", code, nil, nil)
	seedConn(t, f, code, "sa_conn_ok_"+shortID(t), nil)

	out, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
	require.NoError(t, err)
	sa := f.jobSA(t, out.Body.JobID)
	require.NotNil(t, sa)
	assert.Equal(t, "sa_conn_ok_"+shortID(t), *sa)
	job, _ := f.s.Jobs.FindByID(context.Background(), out.Body.JobID)
	assert.Contains(t, job.Metadata, dispatchjob.Metadata{Key: "fn.connection", Value: code})
	assert.Nil(t, f.jobPool(t, out.Body.JobID), "no pool declared: client default, independent of the connection")
}

func TestDispatchConnection_NoneDeclaredIsUnchanged(t *testing.T) {
	t.Parallel()
	f := newDeclFixture(t, "", "", nil, nil)
	seedConn(t, f, connCode(t, "unused"), "sa_unused_"+shortID(t), nil)
	out, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
	require.NoError(t, err)
	assert.Nil(t, f.jobSA(t, out.Body.JobID), "today's behaviour: the application's account signs")
}

func TestDispatchConnection_PoolAndConnectionAreIndependent(t *testing.T) {
	t.Parallel()
	pc, cc := poolCode(t, "both"), connCode(t, "both")
	f := newDeclFixture(t, pc, cc, nil, nil)
	p := seedPool(t, f, pc, nil)
	seedConn(t, f, cc, "sa_both_"+shortID(t), nil)
	out, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
	require.NoError(t, err)
	require.NotNil(t, f.jobPool(t, out.Body.JobID))
	assert.Equal(t, p.ID, *f.jobPool(t, out.Body.JobID))
	require.NotNil(t, f.jobSA(t, out.Body.JobID))
}

func TestDispatchConnection_Unknown(t *testing.T) {
	t.Parallel()
	f := newDeclFixture(t, "", connCode(t, "ghost"), nil, nil)
	_, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "CONNECTION_NOT_FOUND")
	assert.Zero(t, f.jobsFrom(t))
}

func TestDispatchConnection_PausedIsRefused(t *testing.T) {
	t.Parallel()
	code := connCode(t, "paused")
	f := newDeclFixture(t, "", code, nil, nil)
	seedConn(t, f, code, "sa_paused_"+shortID(t), func(c *connection.Connection) { c.Status = connection.StatusPaused })
	_, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "CONNECTION_NOT_ALLOWED")
	assert.Zero(t, f.jobsFrom(t))
}

func TestDispatchConnection_ClientRules(t *testing.T) {
	t.Parallel()
	a, b := "cli_a_"+shortID(t), "cli_b_"+shortID(t)
	inA := func(fn *function.Function) { fn.ClientID = &a }

	t.Run("a client function may use a client-less connection", func(t *testing.T) {
		code := connCode(t, "shared")
		f := newDeclFixture(t, "", code, inA, inA)
		seedConn(t, f, code, "sa_shared_"+shortID(t), nil)
		_, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
		require.NoError(t, err)
	})
	t.Run("its own client's connection wins over a client-less one", func(t *testing.T) {
		code := connCode(t, "own")
		f := newDeclFixture(t, "", code, inA, inA)
		seedConn(t, f, code, "sa_less_"+shortID(t), nil)
		seedConn(t, f, code, "sa_own_"+shortID(t), func(c *connection.Connection) { c.ClientID = ids.PtrOf[ids.ClientID](&a) })
		out, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
		require.NoError(t, err)
		require.NotNil(t, f.jobSA(t, out.Body.JobID))
		assert.Equal(t, "sa_own_"+shortID(t), *f.jobSA(t, out.Body.JobID))
	})
	t.Run("another client's connection is refused", func(t *testing.T) {
		code := connCode(t, "other")
		f := newDeclFixture(t, "", code, inA, inA)
		seedConn(t, f, code, "sa_other_"+shortID(t), func(c *connection.Connection) { c.ClientID = ids.PtrOf[ids.ClientID](&b) })
		_, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
		testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "CONNECTION_NOT_FOUND")
		assert.Zero(t, f.jobsFrom(t))
	})
	t.Run("a client-less function cannot use a client's connection", func(t *testing.T) {
		code := connCode(t, "clientonly")
		f := newDeclFixture(t, "", code, nil, nil)
		seedConn(t, f, code, "sa_conly_"+shortID(t), func(c *connection.Connection) { c.ClientID = ids.PtrOf[ids.ClientID](&b) })
		_, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
		testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "CONNECTION_NOT_FOUND")
	})
	t.Run("a paused own-client connection is refused, not skipped for the shared one", func(t *testing.T) {
		code := connCode(t, "ownpaused")
		f := newDeclFixture(t, "", code, inA, inA)
		seedConn(t, f, code, "sa_shared2_"+shortID(t), nil)
		seedConn(t, f, code, "sa_ownp_"+shortID(t), func(c *connection.Connection) {
			c.ClientID = ids.PtrOf[ids.ClientID](&a)
			c.Status = connection.StatusPaused
		})
		_, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "CONNECTION_NOT_ALLOWED")
	})
}

func TestDispatchConnection_ApplicationNamespace(t *testing.T) {
	t.Parallel()
	t.Run("the function's own application's connection is allowed and wins over a shared one", func(t *testing.T) {
		code := connCode(t, "mine")
		f := newDeclFixture(t, "", code, nil, nil)
		seedConn(t, f, code, "sa_shr_"+shortID(t), nil)
		seedConn(t, f, code, "sa_app_"+shortID(t), func(c *connection.Connection) { c.ApplicationCode = new(f.creator.ApplicationCode) })
		out, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
		require.NoError(t, err)
		require.NotNil(t, f.jobSA(t, out.Body.JobID))
		assert.Equal(t, "sa_app_"+shortID(t), *f.jobSA(t, out.Body.JobID))
	})
	t.Run("a connection owned by another application is refused", func(t *testing.T) {
		code := connCode(t, "theirs")
		f := newDeclFixture(t, "", code, nil, nil)
		seedConn(t, f, code, "sa_theirs_"+shortID(t), func(c *connection.Connection) { c.ApplicationCode = new("some-other-app") })
		_, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
		testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "CONNECTION_NOT_FOUND")
		assert.Zero(t, f.jobsFrom(t))
	})
}

// Through the real route a `connection` in the dispatch body changes nothing.
func TestDispatchConnection_RequestCannotOverrideTheDeclaration(t *testing.T) {
	t.Parallel()
	declared, other := connCode(t, "declared"), connCode(t, "wanted")
	f := newDeclFixture(t, "", declared, nil, nil)
	seedConn(t, f, declared, "sa_decl_"+shortID(t), nil)
	seedConn(t, f, other, "sa_want_"+shortID(t), nil)

	ti := newTestTokenIssuer(t)
	srv := newTestServer(t, f.s, ti)
	runner := ti.mint(t, "sa_fn_runner_dsconn", []string{"platform:function:runner:control"})
	raw, err := json.Marshal(map[string]any{
		"functionId": f.creator.ID, "version": 2,
		"dispatch": map[string]any{"target": f.app + ".logger", "path": "/deliver", "connection": other},
		"payload":  "e30=",
	})
	require.NoError(t, err)
	resp := postJSON(t, srv.URL+fncontrol.PathDispatchJobs, runner, raw)
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var out fncontrol.DispatchResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
		require.NotNil(t, f.jobSA(t, out.JobID))
		assert.Equal(t, "sa_decl_"+shortID(t), *f.jobSA(t, out.JobID))
	} else {
		assert.Zero(t, f.jobsFrom(t))
	}
}

func TestDispatchConnection_UnwiredRepositoryIsAnInternalError(t *testing.T) {
	t.Parallel()
	f := newDeclFixture(t, "", connCode(t, "unwired"), nil, nil)
	f.s.Connections = nil
	_, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
	testpg.RequireUsecaseError(t, err, usecase.KindInternal, "DISPATCH_UNWIRED")
}
