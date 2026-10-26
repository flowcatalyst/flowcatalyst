package runtimeapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/tsid"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// SecretFetchEvent is the event name (the message and the "event" field) of
// the security log line every runtime secret fetch writes: INFO for a fetch
// that answered the value, WARN for a refused one. A log pipeline keys on it.
const SecretFetchEvent = "security.ingestion_secret_fetch"

// SecretFetchRefusedOperation is the aud_logs operation of a refused fetch.
// A successful fetch writes no audit row (owner ruling 2026-10-25); a refused
// one does, on the ingestion, so it reads in the ingestion's history.
const SecretFetchRefusedOperation = "SECRET_FETCH_REFUSED"

// secretsSection is the platform config section an ingestion's secrets are
// kept in, at the ingestion's application and client.
const secretsSection = "secrets"

type getSecretInput struct {
	ID        string `path:"id"`
	Name      string `path:"name" pattern:"^[A-Za-z0-9._:-]{1,201}$" doc:"A secret the ingestion's live definition names"`
	RuntimeID string `query:"runtimeId" required:"true" pattern:"^[A-Za-z0-9._:-]{1,100}$" doc:"The asking runtime, which must carry the ingestion's kind and for a polled source hold its lease"`
}

type secretOutput struct {
	// CacheControl keeps the plaintext out of every shared or browser cache.
	CacheControl string `header:"Cache-Control"`
	Body         SecretValue
}

// secretFetch is what one fetch found, for the telemetry: as much as was
// known when it answered or was refused.
type secretFetch struct {
	ingestion       *ingestion.Ingestion
	applicationCode string
}

// getSecret answers a secret the ingestion's live definition binds, from the
// ingestion's application, section "secrets" and client with no global
// fallback. It is not a config reveal (owner ruling 2026-10-25): the runtime
// holds no reveal permission and needs none, and a successful fetch writes no
// aud_logs row (a runtime re-reads each secret about once a minute per
// route). It is security telemetry instead: a "security.ingestion_secret_fetch"
// log line, the fc_ingestion_secret_fetch_total counter and the
// (ingestion, secret, runtime) last-fetched record the pages and
// GET /api/ingestions/{id}/secrets show. A refused fetch (a name the live
// definition does not bind, no value at the ingestion's client, no lease,
// the kind not carried, a paused ingestion, an inactive client, a caller that
// is not a runtime) is audited as SECRET_FETCH_REFUSED and logged at WARN.
func (s *State) getSecret(ctx context.Context, in *getSecretInput) (*secretOutput, error) {
	ac := auth.FromContext(ctx)
	f, out, err := s.fetchSecret(ctx, in)
	if err != nil {
		s.secretFetchFailed(ctx, ac, in, f, err)
		return nil, err
	}
	i := f.ingestion
	principal := principalOf(ac)
	ingestion.CountSecretFetch(ingestion.SecretFetchSuccess)
	slog.InfoContext(ctx, SecretFetchEvent, "event", SecretFetchEvent, "outcome", ingestion.SecretFetchSuccess,
		"ingestionId", i.ID, "applicationCode", f.applicationCode, "clientId", string(i.ClientID),
		"secretName", in.Name, "runtimeId", in.RuntimeID, "principalId", principal, "version", i.Version)
	if err := s.Repo.RecordSecretFetch(ctx, i.ID, in.Name, in.RuntimeID, i.Version, principal); err != nil {
		// The answer stands: the record is what the pages show of the fetch,
		// which the log line above has already recorded.
		slog.WarnContext(ctx, "ingestion secret fetch not recorded", "ingestionId", i.ID, "secretName", in.Name,
			"runtimeId", in.RuntimeID, "err", err)
	}
	return out, nil
}

// fetchSecret runs the checks and reads the value. f carries what was found
// before a refusal.
func (s *State) fetchSecret(ctx context.Context, in *getSecretInput) (secretFetch, *secretOutput, error) {
	var f secretFetch
	i, err := s.requireRuntimeIngestion(ctx, in.ID, in.RuntimeID, true)
	f.ingestion = i
	if err != nil {
		return f, nil, err
	}
	app, err := s.Resolver.ApplicationByID(ctx, string(i.ApplicationID))
	if err != nil {
		return f, nil, err
	}
	if app == nil {
		return f, nil, usecase.Internal("REPO", "ingestion's application not found", nil)
	}
	f.applicationCode = app.Code
	// The names are those of the effective live source: when the settings are
	// in a config entry, the entry's value is what binds a secret.
	effective, err := s.effective(ctx, app.Code, i)
	if err != nil {
		return f, nil, err
	}
	if !slices.Contains(ingestion.SecretNames(effective.Definition.Source), in.Name) {
		return f, nil, usecase.NotFound("SECRET_NOT_FOUND", "the ingestion's live definition binds no secret "+in.Name)
	}
	r, err := s.Config.FetchBoundSecretForIngestionRuntime(ctx, app.Code, secretsSection, in.Name, string(i.ClientID))
	if e := usecase.AsError(err); e != nil && e.Kind == usecase.KindNotFound {
		return f, nil, usecase.NotFound("SECRET_NOT_FOUND", "the secret "+in.Name+" has no value at the ingestion's client")
	}
	if err != nil {
		return f, nil, err
	}
	return f, &secretOutput{CacheControl: "no-store", Body: SecretValue{Value: r.Value, UpdatedAt: jsontime.New(r.Config.UpdatedAt)}}, nil
}

// secretFetchFailed counts and logs a fetch that did not answer the value.
// A refusal (any error the caller is answered with, not a fault of the
// platform's) is also audited, on the ingestion; an audit row that cannot be
// written does not change the answer and is logged.
func (s *State) secretFetchFailed(ctx context.Context, ac *auth.AuthContext, in *getSecretInput, f secretFetch, err error) {
	e := usecase.AsError(err)
	principal := principalOf(ac)
	attrs := []any{
		"event", SecretFetchEvent, "ingestionId", in.ID, "applicationCode", f.applicationCode,
		"secretName", in.Name, "runtimeId", in.RuntimeID, "principalId", principal,
	}
	if f.ingestion != nil {
		attrs = append(attrs, "clientId", string(f.ingestion.ClientID), "version", f.ingestion.Version)
	}
	if e == nil || e.Kind == usecase.KindInternal {
		ingestion.CountSecretFetch(ingestion.SecretFetchError)
		slog.ErrorContext(ctx, SecretFetchEvent, append(attrs, "outcome", ingestion.SecretFetchError, "err", err)...)
		return
	}
	ingestion.CountSecretFetch(ingestion.SecretFetchRefused)
	slog.WarnContext(ctx, SecretFetchEvent, append(attrs, "outcome", ingestion.SecretFetchRefused, "reason", e.Code, "message", e.Message)...)
	s.auditRefusal(ctx, principal, in, f, e)
}

// auditRefusal writes the SECRET_FETCH_REFUSED row of a refused fetch on the
// ingestion it asked about.
func (s *State) auditRefusal(ctx context.Context, principal string, in *getSecretInput, f secretFetch, e *usecase.Error) {
	if s.Audit == nil {
		slog.ErrorContext(ctx, "ingestion secret fetch refusal not audited: the runtime API has no audit log wired",
			"ingestionId", in.ID, "secretName", in.Name, "runtimeId", in.RuntimeID, "reason", e.Code)
		return
	}
	detail := map[string]string{"secretName": in.Name, "runtimeId": in.RuntimeID, "reason": e.Code, "message": e.Message}
	if f.applicationCode != "" {
		detail["applicationCode"] = f.applicationCode
	}
	operationJSON, _ := json.Marshal(detail)
	row := &audit.Log{
		ID:            tsid.Generate(tsid.AuditLog),
		EntityType:    "Ingestion",
		EntityID:      in.ID,
		Operation:     SecretFetchRefusedOperation,
		OperationJSON: operationJSON,
		PerformedAt:   time.Now().UTC(),
	}
	if principal != "" {
		row.PrincipalID = &principal
	}
	if f.ingestion != nil {
		row.ClientID = ids.StringPtr(&f.ingestion.ClientID)
		row.ApplicationID = ids.StringPtr(&f.ingestion.ApplicationID)
	}
	if err := s.Audit.Insert(ctx, row); err != nil {
		slog.ErrorContext(ctx, "ingestion secret fetch refusal not audited", "ingestionId", in.ID, "secretName", in.Name,
			"runtimeId", in.RuntimeID, "reason", e.Code, "err", err)
	}
}

func principalOf(ac *auth.AuthContext) string {
	if ac == nil {
		return ""
	}
	return ac.PrincipalID
}
