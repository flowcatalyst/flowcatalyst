package mcp

import (
	"net/http"

	"github.com/flowcatalyst/flowcatalyst-go/internal/oauthtoken"
)

// TokenManager is the shared client-credentials token manager. It lived here
// until the router needed the same thing to fetch its configuration document;
// the router must not import the MCP server to get it.
type TokenManager = oauthtoken.Manager

// NewTokenManager builds a token manager for the given platform base URL and
// client credentials. httpClient may be nil (http.DefaultClient is used).
func NewTokenManager(baseURL, clientID, clientSecret string, httpClient *http.Client) *TokenManager {
	return oauthtoken.New(baseURL, clientID, clientSecret, httpClient)
}
