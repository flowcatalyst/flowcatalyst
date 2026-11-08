//go:build integration

package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/go-sdk/usecase"
)

func (f *fixture) ensureWithPhone(code, phone string) (*ensureOutput, error) {
	return f.s.ensureWorker(f.actx, &apicommon.In[EnsureFieldWorkerRequest]{Body: EnsureFieldWorkerRequest{
		ClientID: f.clientID, WorkerCode: code, Name: "Phone Worker", Phone: &phone, AppCodes: []string{f.codeA},
	}})
}

func (f *fixture) phoneOf(id string) string {
	f.t.Helper()
	out, err := f.s.getWorker(f.actx, &apicommon.IDInput{ID: id})
	require.NoError(f.t, err)
	return out.Body.Phone
}

func TestEnsureWorkerStoresPhoneNewOnly(t *testing.T) {
	f := newFixture(t, hasher(t))
	first, err := f.ensureWithPhone("W-PH-1", "  +27 82 123 4567 ")
	require.NoError(t, err)
	assert.Equal(t, "+27 82 123 4567", first.Body.Phone, "stored trimmed, as entered")
	assert.Equal(t, "+27 82 123 4567", f.phoneOf(first.Body.ID), "persisted")

	// An existing worker keeps its phone.
	again, err := f.ensureWithPhone("W-PH-1", "082-999-9999")
	require.NoError(t, err)
	assert.False(t, *again.Body.Created)
	assert.Equal(t, "+27 82 123 4567", again.Body.Phone)
	assert.Equal(t, "+27 82 123 4567", f.phoneOf(first.Body.ID))

	// No phone at all stays empty.
	none := f.ensure(f.actx, "W-PH-2", "No Phone", f.codeA)
	assert.Empty(t, none.Body.Phone)
}

func TestUpdateWorkerPhoneSetClearUnchanged(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := f.ensure(f.actx, "W-PH-3", "Upd Phone", f.codeA).Body
	set := func(phone *string) FieldWorkerResponse {
		out, err := f.s.updateWorker(f.actx, &updateWorkerInput{ID: w.ID, Body: UpdateFieldWorkerRequest{Name: "Upd Phone", Phone: phone}})
		require.NoError(t, err)
		return out.Body
	}
	p := "(021) 555 0100"
	assert.Equal(t, p, set(&p).Phone)
	assert.Equal(t, p, f.phoneOf(w.ID))

	assert.Equal(t, p, set(nil).Phone, "omitted leaves it unchanged")
	assert.Equal(t, p, f.phoneOf(w.ID))

	empty := ""
	assert.Empty(t, set(&empty).Phone, "empty string clears it")
	assert.Empty(t, f.phoneOf(w.ID))
}

func TestPhoneInvalidIs400(t *testing.T) {
	f := newFixture(t, hasher(t))
	_, err := f.ensureWithPhone("W-PH-4", "12345")
	requireErr(t, err, usecase.KindValidation, "PHONE_INVALID")
	assert.Equal(t, 400, httperror.Status(err))

	w := f.ensure(f.actx, "W-PH-5", "Bad Upd", f.codeA).Body
	bad := "082 123 4567 ext"
	_, err = f.s.updateWorker(f.actx, &updateWorkerInput{ID: w.ID, Body: UpdateFieldWorkerRequest{Name: "Bad Upd", Phone: &bad}})
	requireErr(t, err, usecase.KindValidation, "PHONE_INVALID")
	assert.Equal(t, 400, httperror.Status(err))
	assert.Empty(t, f.phoneOf(w.ID))
}
