//go:build integration

package api

import (
	"context"
	"fmt"
	"math/rand/v2"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
)

var generatedCode = func(digits int) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(`^[%s]{3}[%s]{%d}$`,
		fieldidentity.WorkerCodeLetters, fieldidentity.WorkerCodeDigits, digits))
}

func (f *fixture) create(ctx context.Context, body EnsureFieldWorkerRequest) (*ensureOutput, error) {
	f.t.Helper()
	body.ClientID = f.clientID
	if body.Name == "" {
		body.Name = "Generated"
	}
	return f.s.ensureWorker(ctx, &apicommon.In[EnsureFieldWorkerRequest]{Body: body})
}

func (f *fixture) workerCount() int {
	f.t.Helper()
	var n int
	require.NoError(f.t, testpg.Pool(f.t).QueryRow(context.Background(),
		`SELECT COUNT(*) FROM field_identities WHERE client_id = $1`, f.clientID).Scan(&n))
	return n
}

func (f *fixture) setDigits(n int) {
	f.t.Helper()
	_, err := f.s.putPolicy(f.actx, &putPolicyInput{ClientID: f.clientID, Body: PutFieldPolicyRequest{WorkerCodeDigits: &n}})
	require.NoError(f.t, err)
}

// scripted returns the codes in order, then repeats the last; it counts draws.
func scripted(draws *atomic.Int64, codes ...string) func(int) (string, error) {
	return func(int) (string, error) {
		n := draws.Add(1)
		return codes[min(int(n)-1, len(codes)-1)], nil
	}
}

func TestEnsureWithoutCodeGeneratesOne(t *testing.T) {
	f := newFixture(t, hasher(t))
	out, err := f.create(f.actx, EnsureFieldWorkerRequest{Name: "Kim Picker", AppCodes: []string{f.codeA}})
	require.NoError(t, err)
	assert.Equal(t, 201, out.Status)
	require.NotNil(t, out.Body.Created)
	assert.True(t, *out.Body.Created)
	assert.Regexp(t, generatedCode(3), out.Body.WorkerCode, "default policy: three letters, three digits")
	require.Len(t, out.Body.Apps, 1)

	stored, err := f.s.Ops.R.Identities.FindByClientAndCode(context.Background(), f.clientID, out.Body.WorkerCode)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, out.Body.ID, stored.ID)
	// Sign-in looks the code up case-insensitively, as a typed one.
	lower, err := f.s.Ops.R.Identities.FindByClientAndCode(context.Background(), f.clientID, " "+strings.ToLower(out.Body.WorkerCode)+" ")
	require.NoError(t, err)
	require.NotNil(t, lower)
	assert.Equal(t, out.Body.ID, lower.ID)

	var generated bool
	require.NoError(t, testpg.Pool(t).QueryRow(context.Background(),
		`SELECT (data ->> 'workerCodeGenerated')::boolean FROM msg_events WHERE type = $1 AND subject LIKE '%' || $2`,
		fieldidentity.WorkerCreatedType, out.Body.ID).Scan(&generated))
	assert.True(t, generated, "the created event says the code was generated")

	// A supervisor-entered PIN works with a generated code.
	pin := "482915"
	withPin, err := f.create(f.actx, EnsureFieldWorkerRequest{Pin: &pin})
	require.NoError(t, err)
	assert.True(t, withPin.Body.HasPin)
	assert.Regexp(t, generatedCode(3), withPin.Body.WorkerCode)
}

func TestWorkerCodeDigitsSettingAppliesToLaterCodesOnly(t *testing.T) {
	f := newFixture(t, hasher(t))
	before, err := f.create(f.actx, EnsureFieldWorkerRequest{})
	require.NoError(t, err)
	require.Regexp(t, generatedCode(3), before.Body.WorkerCode)

	f.setDigits(6)
	got, err := f.s.getPolicy(f.actx, &policyInput{ClientID: f.clientID})
	require.NoError(t, err)
	assert.Equal(t, 6, got.Body.WorkerCodeDigits)

	after, err := f.create(f.actx, EnsureFieldWorkerRequest{})
	require.NoError(t, err)
	assert.Regexp(t, generatedCode(6), after.Body.WorkerCode)

	again, err := f.s.Ops.R.Identities.FindByID(context.Background(), before.Body.ID)
	require.NoError(t, err)
	assert.Equal(t, before.Body.WorkerCode, again.WorkerCode, "an existing code is never changed by the setting")
}

func TestWorkerCodeDigitsPolicyValidation(t *testing.T) {
	f := newFixture(t, hasher(t))
	def, err := f.s.getPolicy(f.actx, &policyInput{ClientID: f.clientID})
	require.NoError(t, err)
	assert.Equal(t, 3, def.Body.WorkerCodeDigits)
	for _, n := range []int{2, 7} {
		_, err := f.s.putPolicy(f.actx, &putPolicyInput{ClientID: f.clientID, Body: PutFieldPolicyRequest{WorkerCodeDigits: &n}})
		requireErr(t, err, usecase.KindValidation, "POLICY_INVALID")
	}
	five := 5
	put, err := f.s.putPolicy(f.actx, &putPolicyInput{ClientID: f.clientID, Body: PutFieldPolicyRequest{WorkerCodeDigits: &five}})
	require.NoError(t, err)
	assert.Equal(t, 5, put.Body.WorkerCodeDigits)
	// A PUT replaces the policy: omitting the field restores the default.
	put, err = f.s.putPolicy(f.actx, &putPolicyInput{ClientID: f.clientID, Body: PutFieldPolicyRequest{}})
	require.NoError(t, err)
	assert.Equal(t, 3, put.Body.WorkerCodeDigits)
}

func TestGeneratedCodeCollisionIsRedrawn(t *testing.T) {
	f := newFixture(t, hasher(t))
	// A typed code in lower case holds the normalised key the draw collides with.
	typed := f.ensure(f.actx, "bcd234", "Typed")
	require.Equal(t, "BCD234", typed.Body.WorkerCode)

	var draws atomic.Int64
	f.s.Ops.NewWorkerCode = scripted(&draws, "BCD234", "bcd234", "BCD235")
	out, err := f.create(f.actx, EnsureFieldWorkerRequest{Name: "Second"})
	require.NoError(t, err)
	assert.Equal(t, "BCD235", out.Body.WorkerCode)
	assert.Equal(t, int64(3), draws.Load(), "both draws of the held code (any case) were redrawn")
	assert.NotEqual(t, typed.Body.ID, out.Body.ID)

	typedAgain, err := f.s.Ops.R.Identities.FindByID(context.Background(), typed.Body.ID)
	require.NoError(t, err)
	assert.Equal(t, "Typed", typedAgain.Name, "the colliding worker is untouched")
	assert.Equal(t, 2, f.workerCount())
	assert.Equal(t, 1, f.eventCount(fieldidentity.WorkerCreatedType, out.Body.ID), "the failed draws recorded nothing")
}

func TestGeneratedCodeSpaceExhausted(t *testing.T) {
	f := newFixture(t, hasher(t))
	f.ensure(f.actx, "KTM482", "Holder")
	var draws atomic.Int64
	f.s.Ops.NewWorkerCode = scripted(&draws, "KTM482")
	_, err := f.create(f.actx, EnsureFieldWorkerRequest{})
	requireErr(t, err, usecase.KindBusinessRule, fieldidentity.CodeWorkerCodesExhausted)
	assert.Equal(t, int64(10), draws.Load(), "a bounded number of draws")
	assert.Equal(t, 1, f.workerCount(), "nothing was created")
}

// A typed code behaves exactly as before, also against a generated code that
// differs only by case: it is the same worker.
func TestTypedCodeUnchangedAgainstGeneratedCodes(t *testing.T) {
	f := newFixture(t, hasher(t))
	var draws atomic.Int64
	f.s.Ops.NewWorkerCode = scripted(&draws, "MNP678")
	gen, err := f.create(f.actx, EnsureFieldWorkerRequest{Name: "Generated"})
	require.NoError(t, err)
	require.Equal(t, "MNP678", gen.Body.WorkerCode)

	typed := f.ensure(f.actx, "mnp678", "Someone Else", f.codeA)
	assert.Equal(t, 200, typed.Status)
	assert.False(t, *typed.Body.Created)
	assert.Equal(t, gen.Body.ID, typed.Body.ID)
	assert.Equal(t, "Generated", typed.Body.Name)
	assert.Equal(t, 1, f.workerCount())

	other := f.ensure(f.actx, "w-77", "Typed")
	assert.Equal(t, "W-77", other.Body.WorkerCode)
	assert.Equal(t, int64(1), draws.Load(), "a typed code draws nothing")
}

func TestConcurrentGenerationForOneClient(t *testing.T) {
	f := newFixture(t, hasher(t))
	// A small pool of 200 codes makes collisions between the concurrent
	// creations likely; every one must still succeed with its own code.
	pool := make([]string, 200)
	for n := range pool {
		pool[n] = fmt.Sprintf("QRS%03d", 200+n)
	}
	var draws atomic.Int64
	f.s.Ops.NewWorkerCode = func(int) (string, error) {
		draws.Add(1)
		return pool[rand.IntN(len(pool))], nil //nolint:gosec // test data, not a secret
	}
	const workers = 40
	codes := make([]string, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for n := range workers {
		wg.Go(func() {
			out, err := f.create(f.actx, EnsureFieldWorkerRequest{Name: fmt.Sprintf("W%d", n)})
			errs[n] = err
			if err == nil {
				codes[n] = out.Body.WorkerCode
			}
		})
	}
	wg.Wait()
	assert.Greater(t, draws.Load(), int64(workers), "some draws collided and were redrawn")
	seen := map[string]bool{}
	for n := range workers {
		require.NoError(t, errs[n], "creation %d", n)
		assert.False(t, seen[codes[n]], "code %s given twice", codes[n])
		seen[codes[n]] = true
	}
	assert.Equal(t, workers, f.workerCount())
}

func TestConcurrentGenerationRealGenerator(t *testing.T) {
	f := newFixture(t, hasher(t))
	const workers = 50
	codes := make([]string, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for n := range workers {
		wg.Go(func() {
			out, err := f.create(f.actx, EnsureFieldWorkerRequest{Name: fmt.Sprintf("W%d", n)})
			errs[n] = err
			if err == nil {
				codes[n] = out.Body.WorkerCode
			}
		})
	}
	wg.Wait()
	seen := map[string]bool{}
	for n := range workers {
		require.NoError(t, errs[n])
		assert.Regexp(t, generatedCode(3), codes[n])
		assert.False(t, seen[codes[n]])
		seen[codes[n]] = true
	}
	assert.Equal(t, workers, f.workerCount())
}

func TestIdempotencyKeyMakesCreationRepeatable(t *testing.T) {
	f := newFixture(t, hasher(t))
	first, err := f.create(f.actx, EnsureFieldWorkerRequest{Name: "Ann", IdempotencyKey: "hr-1001", AppCodes: []string{f.codeA}})
	require.NoError(t, err)
	assert.Equal(t, 201, first.Status)

	// A repeat (a retry after a lost answer) answers the same worker and only
	// adds grants, as an ensure by code does.
	again, err := f.create(f.actx, EnsureFieldWorkerRequest{Name: "Ann (retry)", IdempotencyKey: "hr-1001", AppCodes: []string{f.codeB}})
	require.NoError(t, err)
	assert.Equal(t, 200, again.Status)
	assert.False(t, *again.Body.Created)
	assert.Equal(t, first.Body.ID, again.Body.ID)
	assert.Equal(t, first.Body.WorkerCode, again.Body.WorkerCode)
	assert.Equal(t, "Ann", again.Body.Name)
	assert.Len(t, again.Body.Apps, 2)
	assert.Equal(t, 1, f.workerCount())

	// Another key is another worker; a key is per client.
	other, err := f.create(f.actx, EnsureFieldWorkerRequest{IdempotencyKey: "hr-1002"})
	require.NoError(t, err)
	assert.NotEqual(t, first.Body.ID, other.Body.ID)
	g := newFixture(t, hasher(t))
	elsewhere, err := g.create(g.actx, EnsureFieldWorkerRequest{IdempotencyKey: "hr-1001"})
	require.NoError(t, err)
	assert.True(t, *elsewhere.Body.Created)

	_, err = f.create(f.actx, EnsureFieldWorkerRequest{WorkerCode: "W-1", IdempotencyKey: "hr-1003"})
	requireErr(t, err, usecase.KindValidation, "IDEMPOTENCY_KEY_WITH_WORKER_CODE")
}

func TestIdempotencyKeyUnderConcurrency(t *testing.T) {
	f := newFixture(t, hasher(t))
	const callers = 12
	ids := make([]string, callers)
	created := make([]bool, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for n := range callers {
		wg.Go(func() {
			out, err := f.create(f.actx, EnsureFieldWorkerRequest{IdempotencyKey: "same-key"})
			errs[n] = err
			if err == nil {
				ids[n], created[n] = out.Body.ID, *out.Body.Created
			}
		})
	}
	wg.Wait()
	made := 0
	for n := range callers {
		require.NoError(t, errs[n])
		assert.Equal(t, ids[0], ids[n], "every repeat answers the one worker")
		if created[n] {
			made++
		}
	}
	assert.Equal(t, 1, made, "exactly one call created it")
	assert.Equal(t, 1, f.workerCount())
}

// Without a key a creation without a code is not repeatable: each call is a
// new worker. The SDKs therefore never resend it on an unknown outcome.
func TestCreationWithoutKeyIsNotRepeatable(t *testing.T) {
	f := newFixture(t, hasher(t))
	a, err := f.create(f.actx, EnsureFieldWorkerRequest{Name: "Same"})
	require.NoError(t, err)
	b, err := f.create(f.actx, EnsureFieldWorkerRequest{Name: "Same"})
	require.NoError(t, err)
	assert.NotEqual(t, a.Body.ID, b.Body.ID)
	assert.Equal(t, 2, f.workerCount())
}
