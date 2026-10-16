package oauthapi

import (
	"context"
	"net/http"
	"reflect"

	"github.com/danielgtaylor/huma/v2"
)

// The OAuth/OIDC endpoints are plain chi handlers (form-encoded bodies,
// RFC-shaped errors), so they never reach the huma registry. RegisterSpec
// describes the ones application SDKs call to a huma API for tools/dump-spec
// ONLY. It must never be called from the server wiring: the routes are
// already mounted by the Register*Routes methods and the stub handlers here
// do nothing.
//
// Not described: /oauth/authorize and /auth/oidc/session/end (browser
// redirect flows, not request/response calls).

// TokenForm is the application/x-www-form-urlencoded body of POST
// /oauth/token (parseTokenRequest). Client credentials may instead be sent
// as an HTTP Basic Authorization header (client_secret_basic).
type TokenForm struct {
	GrantType    string `json:"grant_type" enum:"authorization_code,refresh_token,client_credentials" doc:"OAuth grant type"`
	Code         string `json:"code,omitempty" doc:"authorization_code grant: the authorization code"`
	RedirectURI  string `json:"redirect_uri,omitempty" doc:"authorization_code grant: the redirect URI used at /oauth/authorize"`
	ClientID     string `json:"client_id,omitempty" doc:"Client id (omit when using HTTP Basic)"`
	ClientSecret string `json:"client_secret,omitempty" doc:"Client secret (omit when using HTTP Basic)"`
	CodeVerifier string `json:"code_verifier,omitempty" doc:"authorization_code grant: PKCE verifier"`
	RefreshToken string `json:"refresh_token,omitempty" doc:"refresh_token grant: the refresh token"`
	Scope        string `json:"scope,omitempty"`
}

// TokenIntrospectForm is the form body of POST /oauth/introspect and
// POST /oauth/revoke. The caller authenticates with a Bearer access token,
// HTTP Basic client credentials, or client_id/client_secret.
type TokenIntrospectForm struct {
	Token        string `json:"token"`
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
}

// OAuthErrorBody is the RFC 6749 error body written by writeOAuthError.
type OAuthErrorBody struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

type specNoInput struct{}

type (
	specTokenOutput      struct{ Body TokenResponse }
	specIntrospectOutput struct{ Body IntrospectResponse }
	specUserInfoOutput   struct{ Body UserInfoResponse }
	specDiscoveryOutput  struct{ Body OIDCDiscoveryDocument }
	specJWKSOutput       struct{ Body JWKSResponse }
	specEmptyOutput      struct{}
)

func formBody(api huma.API, name string, t reflect.Type) *huma.RequestBody {
	schema := api.OpenAPI().Components.Schemas.Schema(t, true, name)
	return &huma.RequestBody{
		Required: true,
		Content: map[string]*huma.MediaType{
			"application/x-www-form-urlencoded": {Schema: schema},
		},
	}
}

func oauthErrors(api huma.API) map[string]*huma.Response {
	schema := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[OAuthErrorBody](), true, "OAuthErrorBody")
	body := map[string]*huma.MediaType{"application/json": {Schema: schema}}
	return map[string]*huma.Response{
		"400": {Description: "invalid_request, invalid_grant, unsupported_grant_type or unauthorized_client", Content: body},
		"401": {Description: "invalid_client or invalid_token", Content: body},
	}
}

// RegisterSpec documents the OAuth/OIDC endpoints SDKs call. Spec only; see
// the note above.
func RegisterSpec(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "oauthToken",
		Method:      http.MethodPost,
		Path:        "/oauth/token",
		Summary:     "OAuth 2.0 token endpoint (client_credentials, authorization_code, refresh_token)",
		Tags:        []string{"oauth"},
		RequestBody: formBody(api, "TokenForm", reflect.TypeFor[TokenForm]()),
		Responses:   oauthErrors(api),
	}, func(context.Context, *specNoInput) (*specTokenOutput, error) { return nil, nil })

	huma.Register(api, huma.Operation{
		OperationID: "oauthIntrospect",
		Method:      http.MethodPost,
		Path:        "/oauth/introspect",
		Summary:     "RFC 7662 token introspection (always 200; {active:false} for an invalid token)",
		Tags:        []string{"oauth"},
		RequestBody: formBody(api, "TokenIntrospectForm", reflect.TypeFor[TokenIntrospectForm]()),
		Responses:   oauthErrors(api),
	}, func(context.Context, *specNoInput) (*specIntrospectOutput, error) { return nil, nil })

	huma.Register(api, huma.Operation{
		OperationID:   "oauthRevoke",
		Method:        http.MethodPost,
		Path:          "/oauth/revoke",
		Summary:       "RFC 7009 token revocation (always 200, even for an unknown token)",
		Tags:          []string{"oauth"},
		DefaultStatus: http.StatusOK,
		RequestBody:   formBody(api, "TokenIntrospectForm", reflect.TypeFor[TokenIntrospectForm]()),
		Responses:     oauthErrors(api),
	}, func(context.Context, *specNoInput) (*specEmptyOutput, error) { return nil, nil })

	huma.Register(api, huma.Operation{
		OperationID: "oauthUserinfo",
		Method:      http.MethodGet,
		Path:        "/oauth/userinfo",
		Summary:     "OIDC UserInfo for the Bearer access token (POST is accepted too)",
		Tags:        []string{"oauth"},
		Responses:   oauthErrors(api),
	}, func(context.Context, *specNoInput) (*specUserInfoOutput, error) { return nil, nil })

	huma.Register(api, huma.Operation{
		OperationID: "openidConfiguration",
		Method:      http.MethodGet,
		Path:        "/.well-known/openid-configuration",
		Summary:     "OIDC discovery document",
		Tags:        []string{"oauth"},
	}, func(context.Context, *specNoInput) (*specDiscoveryOutput, error) { return nil, nil })

	huma.Register(api, huma.Operation{
		OperationID: "jwks",
		Method:      http.MethodGet,
		Path:        "/.well-known/jwks.json",
		Summary:     "JSON Web Key Set for verifying access and id tokens",
		Tags:        []string{"oauth"},
	}, func(context.Context, *specNoInput) (*specJWKSOutput, error) { return nil, nil })
}
