package fieldidentity

import (
	"context"
	"crypto/rand"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/attestation"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/pin"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// CodeUnconfigured is the error code of every operation that needs the PIN
// pepper when FLOWCATALYST_APP_KEY is not set. The admin API answers it 503.
const CodeUnconfigured = "FIELD_IDENTITY_UNCONFIGURED"

// EnrolmentCodeTTL is how long an enrolment code stays valid.
const EnrolmentCodeTTL = 10 * time.Minute

// enrolmentAlphabet is unambiguous: no 0/O, 1/I/L.
const enrolmentAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

// EnrolmentCodeLength is the number of characters in an enrolment code.
const EnrolmentCodeLength = 10

// NewEnrolmentCodeValue returns a fresh random enrolment code.
func NewEnrolmentCodeValue() (string, error) {
	n := big.NewInt(int64(len(enrolmentAlphabet)))
	b := make([]byte, EnrolmentCodeLength)
	for i := range b {
		d, err := rand.Int(rand.Reader, n)
		if err != nil {
			return "", err
		}
		b[i] = enrolmentAlphabet[d.Int64()]
	}
	return string(b), nil
}

// Ops builds the field-identity domain operations over their dependencies.
// Every operation is client-boxed in its Authorize phase (creates) or right
// after loading its target (by-id operations); application-confined callers
// (service accounts holding an explicit application list) are limited to
// their own applications. The coarse permission check stays in the
// controller, per the locked authorization model.
type Ops struct {
	R       *Repos
	Clients *client.Repository
	Apps    *application.Repository
	Hasher  *pin.Hasher
	// Principals resolves the platform user a worker is linked to.
	Principals *principal.Repository
	// NewWorkerCode draws a worker code with the given number of digits; nil
	// is [GenerateWorkerCode]. Tests replace it to force collisions.
	NewWorkerCode func(digits int) (string, error)
	// AttestationRoots are the trusted roots of an unlock key's attestation
	// (FC_FIELD_ATTESTATION_ROOTS); nil trusts none, and then a policy cannot
	// require attestation (docs/field-unlock-keys-spec.md section 6.3).
	AttestationRoots *attestation.Roots
	// AttestationStatus is the attestation key status list's cache, refreshed in
	// the background; nil switches the revocation check off
	// (FC_FIELD_ATTESTATION_STATUS_URL empty, or no roots configured).
	AttestationStatus *attestation.StatusCache
}

// attestationOptions is what a chain is evaluated against at now.
func (o *Ops) attestationOptions(now time.Time) attestation.Options {
	opts := attestation.Options{Roots: o.AttestationRoots, Now: now}
	if o.AttestationStatus == nil {
		opts.StatusOff = true
	} else if l, ok := o.AttestationStatus.Usable(now); ok {
		opts.Status = l
	}
	return opts
}

func (o *Ops) drawWorkerCode(digits int) (string, error) {
	if o.NewWorkerCode != nil {
		return o.NewWorkerCode(digits)
	}
	return GenerateWorkerCode(digits)
}

// ── shared helpers ───────────────────────────────────────────────────────

func unconfigured() error {
	return usecase.Internal(CodeUnconfigured, "field PINs are unavailable: FLOWCATALYST_APP_KEY is not set", pin.ErrUnconfigured)
}

func pinPolicyError(violations []string) error {
	return usecase.Validation("PIN_POLICY", "the PIN does not meet the client's PIN policy").
		WithDetails(map[string]any{"violations": violations})
}

func repoErr(what string, err error) error {
	return usecase.Internal("REPO", what+" failed", err)
}

// resolvedApp is an application resolved by code.
type resolvedApp struct{ ID, Code string }

// resolveApps resolves codes to applications. Unknown (or, with requireActive,
// inactive) applications are 400 UNKNOWN_APPLICATION; an application the
// caller is not confined to is 403 APPLICATION_FORBIDDEN.
func (o *Ops) resolveApps(ctx context.Context, codes []string, requireActive bool) ([]resolvedApp, error) {
	ac := auth.FromContext(ctx)
	out := make([]resolvedApp, 0, len(codes))
	for _, raw := range codes {
		code := strings.TrimSpace(raw)
		if code == "" {
			continue
		}
		a, err := o.Apps.FindByCode(ctx, code)
		if err == nil && a == nil && strings.ToLower(code) != code {
			a, err = o.Apps.FindByCode(ctx, strings.ToLower(code))
		}
		if err != nil {
			return nil, repoErr("find_application", err)
		}
		if a == nil || (requireActive && !a.Active) {
			return nil, usecase.Validation("UNKNOWN_APPLICATION", "unknown or inactive application: "+code)
		}
		if !ac.CanAccessApplication(string(a.ID)) {
			return nil, usecase.Authorization("APPLICATION_FORBIDDEN", "no access to application: "+a.Code)
		}
		if !slices.ContainsFunc(out, func(r resolvedApp) bool { return r.ID == string(a.ID) }) {
			out = append(out, resolvedApp{ID: string(a.ID), Code: a.Code})
		}
	}
	return out, nil
}

// canActOnWorker enforces application confinement on an existing worker: an
// unconfined caller always may; a confined one needs the worker to hold a
// grant to one of its applications.
func canActOnWorker(ac *auth.AuthContext, i *Identity) error {
	if !ac.IsApplicationScoped() {
		return nil
	}
	if slices.ContainsFunc(i.Apps, func(g AppGrant) bool { return ac.CanAccessApplication(g.AppID) }) {
		return nil
	}
	return usecase.Authorization("APPLICATION_FORBIDDEN", "the worker has no access to any of the caller's applications")
}

func canActOnDevice(ac *auth.AuthContext, d *Device) error {
	if !ac.IsApplicationScoped() {
		return nil
	}
	if slices.ContainsFunc(d.AppIDs, ac.CanAccessApplication) {
		return nil
	}
	return usecase.Authorization("APPLICATION_FORBIDDEN", "the device is not enrolled for any of the caller's applications")
}

// loadWorker loads a worker and applies the resource-level checks.
func (o *Ops) loadWorker(ctx context.Context, id, clientID string) (*Identity, error) {
	ident, err := o.R.Identities.FindByID(ctx, id)
	if err != nil {
		return nil, repoErr("find_field_worker", err)
	}
	if ident == nil || (clientID != "" && ident.ClientID != clientID) {
		return nil, httperror.NotFound("FieldWorker", id)
	}
	ac := auth.FromContext(ctx)
	if err := auth.CheckScopeAccess(ac, &ident.ClientID); err != nil {
		return nil, err
	}
	if err := canActOnWorker(ac, ident); err != nil {
		return nil, err
	}
	return ident, nil
}

func (o *Ops) loadDevice(ctx context.Context, id, clientID string) (*Device, error) {
	d, err := o.R.Devices.FindByID(ctx, id)
	if err != nil {
		return nil, repoErr("find_field_device", err)
	}
	if d == nil || (clientID != "" && d.ClientID != clientID) {
		return nil, httperror.NotFound("FieldDevice", id)
	}
	ac := auth.FromContext(ctx)
	if err := auth.CheckScopeAccess(ac, &d.ClientID); err != nil {
		return nil, err
	}
	if err := canActOnDevice(ac, d); err != nil {
		return nil, err
	}
	return d, nil
}

func appIDs(apps []resolvedApp) []string {
	ids := make([]string, len(apps))
	for n, a := range apps {
		ids[n] = a.ID
	}
	return ids
}

// actorCond selects the sessions a worker holds open as an actor (a supervisor
// acting as another worker).
const actorCond = `(actor_id = $1 AND actor_type = '` + ActorFieldWorker + `')`

// countLive counts live sessions matching cond ($1 = owner id) inside the open
// transaction.
func countLive(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cond, ownerID, appID string) (int, error) {
	n := 0
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		q := `SELECT COUNT(*) FROM field_sessions WHERE (` + cond + `) AND revoked_at IS NULL`
		args := []any{ownerID}
		if appID != "" {
			q += ` AND application_id = $2`
			args = append(args, appID)
		}
		return tx.QueryRow(ctx, q, args...).Scan(&n)
	})
	return n, err
}

func sessionsRevokedEvent(ec usecase.ExecutionContext, clientID, reason string, count int, workerID, deviceID, appID string) Event {
	data := map[string]any{"clientId": clientID, "reason": reason, "count": count}
	id := workerID
	if workerID != "" {
		data["workerId"] = workerID
	}
	if deviceID != "" {
		data["deviceId"] = deviceID
		if id == "" {
			id = deviceID
		}
	}
	if appID != "" {
		data["applicationId"] = appID
	}
	return NewEvent(ec, SessionRevokedType, "field-session", id, data)
}

func requireClient(ctx context.Context, clients *client.Repository, clientID string) error {
	c, err := clients.FindByID(ctx, clientID)
	if err != nil {
		return repoErr("find_client", err)
	}
	if c == nil {
		return httperror.NotFound("Client", clientID)
	}
	return nil
}
