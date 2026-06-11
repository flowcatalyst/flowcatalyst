//go:build integration

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

func anchorCtx() context.Context {
	return auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "p_evt_test",
		Scope:       auth.ScopeAnchor,
	})
}

// eventRow is the persisted msg_events shape compared between the
// singular create and a batch-of-1 (identity/time columns excluded).
type eventRow struct {
	SpecVersion   string
	Type          string
	Source        string
	Subject       *string
	Data          string
	CorrelationID *string
	CausationID   *string
	MessageGroup  *string
	ClientID      *string
	ContextData   string
}

func fetchEventRow(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id string) eventRow {
	t.Helper()
	var row eventRow
	err := pool.QueryRow(ctx,
		`SELECT spec_version, type, source, subject, data::text,
		        correlation_id, causation_id, message_group, client_id,
		        context_data::text
		   FROM msg_events WHERE id = $1`, id).
		Scan(&row.SpecVersion, &row.Type, &row.Source, &row.Subject, &row.Data,
			&row.CorrelationID, &row.CausationID, &row.MessageGroup,
			&row.ClientID, &row.ContextData)
	require.NoError(t, err)
	return row
}

// TestCreateEvent_PersistsAndMatchesBatchOfOne pins that POST /api/events
// persists a row retrievable from msg_events and that the stored row is
// byte-identical in shape to what a batch-of-1 with the same fields
// persists (the singular delegates to the same InsertBatch path).
func TestCreateEvent_PersistsAndMatchesBatchOfOne(t *testing.T) {
	ctx := anchorCtx()
	pool := testpg.Pool(t)
	s := &State{Repo: event.NewRepository(pool)}

	mg, corr, cause := "mg-single", "corr-single", "cause-single"
	out, err := s.create(ctx, &apicommon.In[CreateEventRequest]{Body: CreateEventRequest{
		EventType:       "it:singular:event:created",
		Source:          "test://singular",
		Subject:         "subj-1",
		Data:            json.RawMessage(`{"k":"v"}`),
		MessageGroup:    &mg,
		CorrelationID:   &corr,
		CausationID:     &cause,
		DeduplicationID: "dedup-singular-1",
	}})
	require.NoError(t, err)
	require.NotEmpty(t, out.Body.Event.ID)
	assert.False(t, out.Body.IsDuplicate)
	assert.Zero(t, out.Body.DispatchJobCount)
	assert.Equal(t, "it:singular:event:created", out.Body.Event.EventType)
	assert.Equal(t, "dedup-singular-1", out.Body.Event.DeduplicationID)

	bout, err := s.batchIngest(ctx, &apicommon.In[BatchRequest]{Body: BatchRequest{
		Items: []BatchEventItem{{
			Type:            "it:singular:event:created",
			Source:          "test://singular",
			Subject:         "subj-1",
			Data:            json.RawMessage(`{"k":"v"}`),
			MessageGroup:    &mg,
			CorrelationID:   &corr,
			CausationID:     &cause,
			DeduplicationID: "dedup-batch-1",
		}},
	}})
	require.NoError(t, err)
	require.Len(t, bout.Body.Results, 1)
	require.Equal(t, "SUCCESS", bout.Body.Results[0].Status)

	single := fetchEventRow(t, ctx, pool, out.Body.Event.ID)
	batch := fetchEventRow(t, ctx, pool, bout.Body.Results[0].ID)
	assert.Equal(t, batch, single,
		"singular create must persist the identical row shape as a batch-of-1")
	assert.Equal(t, "1.0", single.SpecVersion)
	assert.JSONEq(t, `{"k":"v"}`, single.Data)
	assert.JSONEq(t, `[]`, single.ContextData)
}

// TestCreateEvent_ContextDataPersisted pins the singular-only contextData
// field round-trips into msg_events.context_data and the response.
func TestCreateEvent_ContextDataPersisted(t *testing.T) {
	ctx := anchorCtx()
	pool := testpg.Pool(t)
	s := &State{Repo: event.NewRepository(pool)}

	out, err := s.create(ctx, &apicommon.In[CreateEventRequest]{Body: CreateEventRequest{
		EventType:   "it:singular:event:ctx",
		Source:      "test://singular-ctx",
		Data:        json.RawMessage(`{}`),
		ContextData: []ContextEntryDTO{{Key: "principalId", Value: "p_ctx"}},
	}})
	require.NoError(t, err)
	require.Len(t, out.Body.Event.ContextData, 1)
	assert.Equal(t, ContextEntryDTO{Key: "principalId", Value: "p_ctx"}, out.Body.Event.ContextData[0])

	row := fetchEventRow(t, ctx, pool, out.Body.Event.ID)
	assert.JSONEq(t, `[{"key":"principalId","value":"p_ctx"}]`, row.ContextData)
}

// TestCreateEvent_ClientDefaultingAndTenantGuard pins the Rust
// create_event client rules: a non-anchor caller without an explicit
// clientId defaults to its first accessible client; an explicit clientId
// outside the caller's tenants is rejected with the FORBIDDEN envelope.
func TestCreateEvent_ClientDefaultingAndTenantGuard(t *testing.T) {
	pool := testpg.Pool(t)
	s := &State{Repo: event.NewRepository(pool)}
	ctx := auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "p_evt_client",
		Scope:       auth.ScopeClient,
		Clients:     []string{"clt_evtsingle1"},
		Permissions: []string{"platform:messaging:batch:events-write"},
	})

	out, err := s.create(ctx, &apicommon.In[CreateEventRequest]{Body: CreateEventRequest{
		EventType: "it:singular:event:tenant",
		Source:    "test://tenant",
		Data:      json.RawMessage(`{"n":1}`),
	}})
	require.NoError(t, err)
	require.NotNil(t, out.Body.Event.ClientID)
	assert.Equal(t, "clt_evtsingle1", *out.Body.Event.ClientID)
	row := fetchEventRow(t, ctx, pool, out.Body.Event.ID)
	require.NotNil(t, row.ClientID)
	assert.Equal(t, "clt_evtsingle1", *row.ClientID)

	other := "clt_evtother"
	_, err = s.create(ctx, &apicommon.In[CreateEventRequest]{Body: CreateEventRequest{
		EventType: "it:singular:event:tenant",
		Source:    "test://tenant",
		Data:      json.RawMessage(`{"n":2}`),
		ClientID:  &other,
	}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "No access to client")
}

// TestCreateEvent_BadPayloadEnvelopeMatchesBatch pins that a bad singular
// payload is rejected with the SAME {error:"VALIDATION", message} envelope
// (and 400 status) the batch endpoint produces — both flow through the
// huma schema validation + httpcompat error model.
func TestCreateEvent_BadPayloadEnvelopeMatchesBatch(t *testing.T) {
	httpcompat.Init()
	_, hapi := humatest.New(t)
	Register(hapi, &State{}) // validation rejects before any handler/repo use

	single := hapi.Post("/api/events", map[string]any{"source": "test://bad"})
	require.Equal(t, http.StatusBadRequest, single.Code, single.Body.String())
	assert.Contains(t, single.Body.String(), `"error":"VALIDATION"`)

	batch := hapi.Post("/api/events/batch", map[string]any{"items": 42})
	require.Equal(t, http.StatusBadRequest, batch.Code, batch.Body.String())
	assert.Contains(t, batch.Body.String(), `"error":"VALIDATION"`)

	var senv, benv struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(single.Body.Bytes(), &senv))
	require.NoError(t, json.Unmarshal(batch.Body.Bytes(), &benv))
	assert.Equal(t, benv.Error, senv.Error, "singular and batch must share the error envelope code")
	assert.True(t, strings.Contains(senv.Message, "validation failed") || senv.Message != "",
		"message must be populated")
}
