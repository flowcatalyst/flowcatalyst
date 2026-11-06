package api

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// ── badges ───────────────────────────────────────────────────────────────

// FieldBadgeResponse is a printed badge on the wire. The token and its hash are
// never part of it: the token is shown once, at issue.
type FieldBadgeResponse struct {
	ID         string           `json:"id"`
	WorkerID   string           `json:"workerId"`
	Label      string           `json:"label,omitempty"`
	Status     string           `json:"status" enum:"ACTIVE,REVOKED"`
	CreatedAt  httpcompat.Time  `json:"createdAt"`
	CreatedBy  string           `json:"createdBy,omitempty"`
	RevokedAt  *httpcompat.Time `json:"revokedAt,omitempty"`
	LastUsedAt *httpcompat.Time `json:"lastUsedAt,omitempty"`
}

func badgeResponse(b *fieldidentity.Badge) FieldBadgeResponse {
	return FieldBadgeResponse{
		ID: b.ID, WorkerID: b.IdentityID, Label: b.Label, Status: string(b.Status),
		CreatedAt: jsontime.New(b.CreatedAt), CreatedBy: b.CreatedBy, RevokedAt: opt(b.RevokedAt), LastUsedAt: opt(b.LastUsedAt),
	}
}

// IssueFieldBadgeRequest is the body of POST /api/field-workers/{id}/badges.
type IssueFieldBadgeRequest struct {
	Label *string `json:"label,omitempty" doc:"A note for the printed badge (at most 255 characters)"`
}

// FieldBadgeIssuedResponse carries the new badge and its token, shown once.
type FieldBadgeIssuedResponse struct {
	Badge FieldBadgeResponse `json:"badge"`
	// Token is the badge token (FWB1 + 26 characters). It is the QR content, is
	// shown once and cannot be retrieved again.
	Token string `json:"token"`
}

type issueBadgeInput struct {
	ID   string `path:"id"`
	Body IssueFieldBadgeRequest
}

func (s *State) issueBadge(ctx context.Context, in *issueBadgeInput) (*apicommon.Out[FieldBadgeIssuedResponse], error) {
	ident, err := s.loadWorker(ctx, in.ID, true)
	if err != nil {
		return nil, err
	}
	token, err := fieldidentity.NewBadgeToken()
	if err != nil {
		return nil, usecase.Internal("TOKEN_GENERATE", "could not generate a badge token", err)
	}
	cmd := fieldidentity.IssueBadgeCommand{WorkerRef: refOf(ident), Token: token}
	if in.Body.Label != nil {
		cmd.Label = *in.Body.Label
	}
	res, err := usecaseop.RunTx(ctx, s.UoW, s.Ops.IssueBadge(), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return nil, fail(err)
	}
	return &apicommon.Out[FieldBadgeIssuedResponse]{Body: FieldBadgeIssuedResponse{Badge: badgeResponse(res.Badge), Token: token}}, nil
}

// FieldBadgeListResponse is the GET /api/field-workers/{id}/badges envelope.
type FieldBadgeListResponse struct {
	Badges []FieldBadgeResponse `json:"badges"`
}

func (s *State) listBadges(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[FieldBadgeListResponse], error) {
	ident, err := s.loadWorker(ctx, in.ID, true)
	if err != nil {
		return nil, err
	}
	rows, err := s.Ops.R.Badges.ListByIdentity(ctx, ident.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "list_field_badges failed", err)
	}
	out := make([]FieldBadgeResponse, 0, len(rows))
	for n := range rows {
		out = append(out, badgeResponse(&rows[n]))
	}
	return &apicommon.Out[FieldBadgeListResponse]{Body: FieldBadgeListResponse{Badges: out}}, nil
}

type revokeBadgeInput struct {
	ID      string `path:"id"`
	BadgeID string `path:"badgeId"`
}

func (s *State) revokeBadge(ctx context.Context, in *revokeBadgeInput) (*apicommon.Out[FieldBadgeResponse], error) {
	ident, err := s.loadWorker(ctx, in.ID, true)
	if err != nil {
		return nil, err
	}
	if _, err := usecaseop.RunTx(ctx, s.UoW, s.Ops.RevokeBadge(),
		fieldidentity.BadgeRef{WorkerRef: refOf(ident), BadgeID: in.BadgeID}, auth.NewExecutionContext(ctx)); err != nil {
		return nil, fail(err)
	}
	b, err := s.Ops.R.Badges.FindByID(ctx, in.BadgeID)
	if err != nil || b == nil {
		return nil, usecase.Internal("REPO", "post-revoke badge lookup failed", err)
	}
	return &apicommon.Out[FieldBadgeResponse]{Body: badgeResponse(b)}, nil
}

// BulkIssueFieldBadgesRequest is the body of POST /api/field-workers/badges.
type BulkIssueFieldBadgesRequest struct {
	WorkerIDs []string `json:"workerIds" minItems:"1" maxItems:"200" doc:"1 to 200 workers, all in clients the caller can access; otherwise nothing is issued"`
}

// BulkIssuedFieldBadge is one issued badge of a bulk issue, with its token.
type BulkIssuedFieldBadge struct {
	WorkerID   string             `json:"workerId"`
	WorkerCode string             `json:"workerCode"`
	Name       string             `json:"name"`
	Badge      FieldBadgeResponse `json:"badge"`
	// Token is shown once and cannot be retrieved again.
	Token string `json:"token"`
}

// BulkIssueFieldBadgesResponse is the outcome of a bulk issue.
type BulkIssueFieldBadgesResponse struct {
	Issued []BulkIssuedFieldBadge `json:"issued"`
}

func (s *State) issueBadges(ctx context.Context, in *apicommon.In[BulkIssueFieldBadgesRequest]) (*apicommon.Out[BulkIssueFieldBadgesResponse], error) {
	ids := in.Body.WorkerIDs
	if len(ids) == 0 || len(ids) > fieldidentity.MaxBulkBadges {
		return nil, usecase.Validation("WORKER_IDS_INVALID", "workerIds must name 1 to 200 workers")
	}
	// Every worker is reachable and manageable by the caller before anything is
	// issued: a foreign or unknown id fails the whole request (403 / 404).
	for _, id := range ids {
		if _, err := s.loadWorker(ctx, id, true); err != nil {
			return nil, err
		}
	}
	tokens := make([]string, len(ids))
	for n := range ids {
		t, err := fieldidentity.NewBadgeToken()
		if err != nil {
			return nil, usecase.Internal("TOKEN_GENERATE", "could not generate a badge token", err)
		}
		tokens[n] = t
	}
	issued, err := usecaseop.RunTx(ctx, s.UoW, s.Ops.IssueBadges(),
		fieldidentity.IssueBadgesCommand{WorkerIDs: ids, Tokens: tokens}, auth.NewExecutionContext(ctx))
	if err != nil {
		return nil, fail(err)
	}
	out := make([]BulkIssuedFieldBadge, 0, len(issued))
	for n, it := range issued {
		out = append(out, BulkIssuedFieldBadge{
			WorkerID: it.Worker.ID, WorkerCode: it.Worker.WorkerCode, Name: it.Worker.Name,
			Badge: badgeResponse(it.Badge), Token: tokens[n],
		})
	}
	return &apicommon.Out[BulkIssueFieldBadgesResponse]{Body: BulkIssueFieldBadgesResponse{Issued: out}}, nil
}

// ── console sign-in codes ────────────────────────────────────────────────

// CreateFieldSignInCodeRequest is the body of POST /api/field-workers/{id}/sign-in-codes.
type CreateFieldSignInCodeRequest struct {
	AppCode string `json:"appCode" doc:"The application the worker is signed in to"`
}

// FieldSignInCodeResponse carries the code, shown once.
type FieldSignInCodeResponse struct {
	// Code is the single-use sign-in code (FWS1 + 26 characters), valid for 10
	// minutes. It is shown once.
	Code      string          `json:"code"`
	ExpiresAt httpcompat.Time `json:"expiresAt"`
	WorkerID  string          `json:"workerId"`
	AppCode   string          `json:"appCode"`
}

type createSignInCodeInput struct {
	ID   string `path:"id"`
	Body CreateFieldSignInCodeRequest
}

// createSignInCode needs the impersonate permission, or manage when the target
// is a test identity (which a service account may also sign in as). The
// permission depends on the worker, so the worker is loaded under a gate that
// looks at it.
func (s *State) createSignInCode(ctx context.Context, in *createSignInCodeInput) (*apicommon.Out[FieldSignInCodeResponse], error) {
	ident, err := s.loadWorkerWith(ctx, in.ID, func(ac *auth.AuthContext, i *fieldidentity.Identity, clientID string) error {
		return auth.CanSignInAsFieldWorker(ac, clientID, i != nil && i.IsTest)
	})
	if err != nil {
		return nil, err
	}
	code, err := fieldidentity.NewSignInCodeValue()
	if err != nil {
		return nil, usecase.Internal("CODE_GENERATE", "could not generate a sign-in code", err)
	}
	res, err := usecaseop.RunTx(ctx, s.UoW, s.Ops.CreateSignInCode(),
		fieldidentity.CreateSignInCodeCommand{WorkerRef: refOf(ident), AppCode: in.Body.AppCode, Code: code},
		auth.NewExecutionContext(ctx))
	if err != nil {
		return nil, fail(err)
	}
	return &apicommon.Out[FieldSignInCodeResponse]{Body: FieldSignInCodeResponse{
		Code: code, ExpiresAt: jsontime.New(res.ExpiresAt), WorkerID: res.WorkerID, AppCode: res.AppCode,
	}}, nil
}
