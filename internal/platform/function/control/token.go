package control

import (
	"context"
	"strconv"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	fncontrol "github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	saops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// FunctionTokenRequest and FunctionTokenResponse are the wire types
// (fncontrol.TokenRequest / TokenResponse) under the names the OpenAPI registry
// knows them by: huma names a schema after its Go type, and oauthapi already
// owns "TokenResponse".
type (
	FunctionTokenRequest  fncontrol.TokenRequest
	FunctionTokenResponse fncontrol.TokenResponse
)

// token serves POST /control/functions/token (the `platform.token` host call,
// ABI op 15; docs/function-platform-token.md): it mints the access token a
// function sends as a bearer to another first-party application, so the
// function holds no client id or secret. Nothing the guest sends decides whose
// token it is:
//
//  1. the invoking version's describe must declare `platformToken: true`;
//  2. the identity is the service account linked to the function's APPLICATION
//     (applications.service_account_id), always. The function's own optional
//     webhook-verifier account (functions.service_account_id) is a different
//     thing and is never used here;
//  3. the account and its principal must be active.
//
// The token is exactly the account's client_credentials grant (same claims,
// scope and lifetime), minted by saops.MintForFunction, which records every
// mint. It is a live credential: never logged.
func (s *State) token(ctx context.Context, in *apicommon.In[FunctionTokenRequest]) (*apicommon.Out[FunctionTokenResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	if err := auth.CanControlFunctionRunner(ac); err != nil {
		return nil, err
	}
	req := fncontrol.TokenRequest(in.Body)
	if strings.TrimSpace(req.FunctionID) == "" {
		return nil, usecase.Validation("FUNCTION_ID_REQUIRED", "functionId is required")
	}

	fn, err := s.Repo.FindByID(ctx, req.FunctionID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if fn == nil {
		return nil, httperror.NotFound("Function", req.FunctionID)
	}
	ver, err := s.Repo.GetVersionByNumber(ctx, fn.ID, int32(req.Version))
	if err != nil {
		return nil, usecase.Internal("REPO", "get_version_by_number failed", err)
	}
	if ver == nil {
		return nil, httperror.NotFound("Function version", fn.Address+"@v"+strconv.Itoa(req.Version))
	}
	desc, err := abi.ParseDescribe(ver.Describe)
	if err != nil {
		return nil, usecase.Internal("CORRUPT_DESCRIBE",
			"function "+fn.Address+" version "+strconv.Itoa(req.Version)+" has an unparseable describe document", err)
	}

	// 1. Declared by THIS version.
	if !desc.PlatformToken {
		return nil, usecase.Authorization("PLATFORM_TOKEN_NOT_DECLARED",
			"this version of "+fn.Address+" does not declare platformToken")
	}

	// 2. The application's own service account.
	if s.Apps == nil {
		return nil, usecase.Internal("TOKEN_UNWIRED", "the application repository is not configured", nil)
	}
	app, err := s.Apps.FindByID(ctx, fn.ApplicationID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find application failed", err)
	}
	accountID, err := s.applicationAccountID(ctx, app)
	if err != nil {
		return nil, err
	}
	if accountID == "" {
		return nil, usecase.BusinessRule("APPLICATION_SERVICE_ACCOUNT_REQUIRED",
			"application "+fn.ApplicationCode+" has no linked service account, so "+fn.Address+" has no identity to call other applications as")
	}

	// 3. Inactive account or principal: SERVICE_ACCOUNT_INACTIVE (the mint's).
	minted, err := saops.MintForFunction(ctx, s.Mint, accountID, fn.Address)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[FunctionTokenResponse]{Body: FunctionTokenResponse{
		AccessToken: minted.AccessToken,
		TokenType:   "Bearer",
		ExpiresIn:   minted.ExpiresIn,
	}}, nil
}

// applicationAccountID is the id of the service account linked to app, or ""
// for none. The link, applications.service_account_id, holds the account's
// SERVICE principal id (it is a foreign key to iam_principals), so the account
// is read off that principal. It is not the account the application's webhook
// deliveries are signed with (iam_service_accounts.application_id, which
// OutboundCredsResolver reads).
func (s *State) applicationAccountID(ctx context.Context, app *application.Application) (string, error) {
	if app == nil || app.ServiceAccountID == nil || *app.ServiceAccountID == "" {
		return "", nil
	}
	if s.Mint.Principals == nil {
		return "", usecase.Internal("TOKEN_UNWIRED", "the principal repository is not configured", nil)
	}
	p, err := s.Mint.Principals.FindByID(ctx, ids.PrincipalID(*app.ServiceAccountID))
	if err != nil {
		return "", usecase.Internal("REPO", "find principal failed", err)
	}
	if p == nil || p.ServiceAccountID == nil {
		return "", nil
	}
	return *p.ServiceAccountID, nil
}
