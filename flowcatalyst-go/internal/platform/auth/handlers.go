package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"log/slog"

	"go.flowcatalyst.tech/internal/common/tsid"
	"go.flowcatalyst.tech/internal/platform/auth/federation"
	"go.flowcatalyst.tech/internal/platform/auth/jwt"
	"go.flowcatalyst.tech/internal/platform/auth/local"
	"go.flowcatalyst.tech/internal/platform/auth/oidc"
	"go.flowcatalyst.tech/internal/platform/auth/session"
	"go.flowcatalyst.tech/internal/platform/client"
	"go.flowcatalyst.tech/internal/platform/principal"
)

// AuthService handles authentication operations
type AuthService struct {
	principalRepo     principal.Repository
	clientRepo        client.Repository
	oidcRepo          *oidc.Repository
	tokenService      *jwt.TokenService
	sessionManager    *session.Manager
	passwordService   *local.PasswordService
	pkceService       *oidc.PKCEService
	federationService *federation.Service
	externalURL       string // External URL for callbacks
}

// NewAuthService creates a new auth service
func NewAuthService(
	principalRepo principal.Repository,
	clientRepo client.Repository,
	oidcRepo *oidc.Repository,
	tokenService *jwt.TokenService,
	sessionManager *session.Manager,
	federationService *federation.Service,
	externalURL string,
) *AuthService {
	return &AuthService{
		principalRepo:     principalRepo,
		clientRepo:        clientRepo,
		oidcRepo:          oidcRepo,
		tokenService:      tokenService,
		sessionManager:    sessionManager,
		passwordService:   local.NewPasswordService(),
		pkceService:       oidc.NewPKCEService(true),
		federationService: federationService,
		externalURL:       externalURL,
	}
}

// === Request/Response types ===

// LoginRequest represents a login request
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse represents a login response
type LoginResponse struct {
	PrincipalID string   `json:"principalId"`
	Name        string   `json:"name"`
	Email       string   `json:"email"`
	Roles       []string `json:"roles"`
	ClientID    string   `json:"clientId,omitempty"`
}

// DomainCheckRequest represents a domain check request
type DomainCheckRequest struct {
	Email string `json:"email"`
}

// DomainCheckResponse represents a domain check response
type DomainCheckResponse struct {
	AuthMethod string `json:"authMethod"` // "internal" or "external"
	LoginURL   string `json:"loginUrl,omitempty"`
	IdpIssuer  string `json:"idpIssuer,omitempty"`
}

// MessageResponse represents a simple message response
type MessageResponse struct {
	Message string `json:"message"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// === Handlers ===

// HandleLogin handles POST /auth/login
func (s *AuthService) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "Email and password are required")
		return
	}

	// Normalize email
	email := local.NormalizeEmail(req.Email)

	// Find principal by email
	p, err := s.principalRepo.FindByEmail(r.Context(), email)
	if err != nil {
		if errors.Is(err, principal.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")
			return
		}
		slog.Error("Failed to find principal", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "Internal server error")
		return
	}

	// Check if principal is active
	if !p.Active {
		writeError(w, http.StatusUnauthorized, "account_disabled", "Account is disabled")
		return
	}

	// Check if this is an internal auth user
	if p.UserIdentity == nil || p.UserIdentity.IdpType != principal.IdpTypeInternal {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "This account uses external authentication")
		return
	}

	// Verify password
	if err := s.passwordService.VerifyPassword(req.Password, p.UserIdentity.PasswordHash); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")
		return
	}

	// Update last login
	_ = s.principalRepo.UpdateLastLogin(r.Context(), p.ID)

	// Get client IDs for token
	clients := s.getClientIDsForPrincipal(r.Context(), p)

	// Extract application codes from roles
	applications := extractApplicationCodes(p.GetRoleNames())

	// Issue session token
	token, err := s.tokenService.IssueSessionToken(
		p.ID,
		p.UserIdentity.Email,
		p.GetRoleNames(),
		clients,
		applications,
	)
	if err != nil {
		slog.Error("Failed to issue session token", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "Failed to create session")
		return
	}

	// Set session cookie
	s.sessionManager.SetSession(w, token)

	// Return login response
	resp := LoginResponse{
		PrincipalID: p.ID,
		Name:        p.Name,
		Email:       p.UserIdentity.Email,
		Roles:       p.GetRoleNames(),
		ClientID:    p.ClientID,
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandleLogout handles POST /auth/logout
func (s *AuthService) HandleLogout(w http.ResponseWriter, r *http.Request) {
	s.sessionManager.ClearSession(w)
	writeJSON(w, http.StatusOK, MessageResponse{Message: "Logged out successfully"})
}

// HandleMe handles GET /auth/me
func (s *AuthService) HandleMe(w http.ResponseWriter, r *http.Request) {
	// Get session token
	token := s.sessionManager.GetSession(r)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Not authenticated")
		return
	}

	// Validate token and get principal ID
	principalID, err := s.tokenService.ValidateSessionToken(token)
	if err != nil {
		s.sessionManager.ClearSession(w)
		writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid or expired session")
		return
	}

	// Find principal
	p, err := s.principalRepo.FindByID(r.Context(), principalID)
	if err != nil {
		s.sessionManager.ClearSession(w)
		writeError(w, http.StatusUnauthorized, "unauthorized", "User not found")
		return
	}

	if !p.Active {
		s.sessionManager.ClearSession(w)
		writeError(w, http.StatusUnauthorized, "account_disabled", "Account is disabled")
		return
	}

	email := ""
	if p.UserIdentity != nil {
		email = p.UserIdentity.Email
	}

	resp := LoginResponse{
		PrincipalID: p.ID,
		Name:        p.Name,
		Email:       email,
		Roles:       p.GetRoleNames(),
		ClientID:    p.ClientID,
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandleCheckDomain handles POST /auth/check-domain
func (s *AuthService) HandleCheckDomain(w http.ResponseWriter, r *http.Request) {
	var req DomainCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}

	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "Email is required")
		return
	}

	// Extract domain from email
	domain := local.ExtractEmailDomain(req.Email)
	if domain == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid email format")
		return
	}

	// Check if this is an anchor domain (internal auth)
	isAnchor, err := s.clientRepo.IsAnchorDomain(r.Context(), domain)
	if err != nil {
		slog.Error("Failed to check anchor domain", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "Internal server error")
		return
	}

	if isAnchor {
		writeJSON(w, http.StatusOK, DomainCheckResponse{
			AuthMethod: "internal",
		})
		return
	}

	// Check for domain-specific auth config
	authConfig, err := s.clientRepo.FindAuthConfigByDomain(r.Context(), domain)
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			// No config found, default to internal
			writeJSON(w, http.StatusOK, DomainCheckResponse{
				AuthMethod: "internal",
			})
			return
		}
		slog.Error("Failed to find auth config", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "Internal server error")
		return
	}

	if authConfig.IsOIDC() {
		writeJSON(w, http.StatusOK, DomainCheckResponse{
			AuthMethod: "external",
			LoginURL:   "/auth/oidc/login?domain=" + domain,
			IdpIssuer:  authConfig.OIDCIssuerURL,
		})
		return
	}

	writeJSON(w, http.StatusOK, DomainCheckResponse{
		AuthMethod: "internal",
	})
}

// === Helper functions ===

func (s *AuthService) getClientIDsForPrincipal(ctx context.Context, p *principal.Principal) []string {
	if p.IsAnchor() {
		return []string{"*"}
	}

	clientIDs := []string{}

	// Add home client
	if p.ClientID != "" {
		clientIDs = append(clientIDs, p.ClientID)
	}

	// Add granted clients
	grants, err := s.clientRepo.FindAccessGrantsByPrincipal(ctx, p.ID)
	if err == nil {
		for _, g := range grants {
			if !g.IsExpired() {
				clientIDs = append(clientIDs, g.ClientID)
			}
		}
	}

	// Deduplicate
	seen := make(map[string]bool)
	result := []string{}
	for _, id := range clientIDs {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}

	return result
}

func extractApplicationCodes(roles []string) []string {
	codes := make(map[string]bool)
	for _, role := range roles {
		// Roles are typically formatted as "application:role-name"
		parts := strings.SplitN(role, ":", 2)
		if len(parts) >= 1 && parts[0] != "" {
			codes[parts[0]] = true
		}
	}

	result := make([]string, 0, len(codes))
	for code := range codes {
		result = append(result, code)
	}
	return result
}

// checkUserApplicationAccess verifies the user has access to at least one
// of the applications the OAuth client is restricted to.
// This matches Java's AuthorizationResource.checkUserApplicationAccess()
func (s *AuthService) checkUserApplicationAccess(p *principal.Principal, oauthClient *oidc.OAuthClient) bool {
	if !oauthClient.HasApplicationRestrictions() {
		return true // No restrictions
	}

	// Anchor users (admin users) have access to all applications
	if p.IsAnchor() {
		return true
	}

	// Get the applications the user has access to via their roles
	userApps := extractApplicationCodes(p.GetRoleNames())

	// Check if any user application matches the client's allowed applications
	for _, userApp := range userApps {
		for _, allowedApp := range oauthClient.ApplicationIDs {
			if userApp == allowedApp {
				return true
			}
		}
	}

	return false
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, err, description string) {
	writeJSON(w, status, ErrorResponse{
		Error:            err,
		ErrorDescription: description,
	})
}

// === OAuth Token Endpoint ===

// TokenRequest represents an OAuth token request
type TokenRequest struct {
	GrantType    string `json:"grant_type"`
	Code         string `json:"code"`
	RedirectURI  string `json:"redirect_uri"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	CodeVerifier string `json:"code_verifier"`
	RefreshToken string `json:"refresh_token"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	Scope        string `json:"scope"`
}

// TokenResponse represents an OAuth token response
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
}

// HandleToken handles POST /oauth/token
func (s *AuthService) HandleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}

	grantType := r.FormValue("grant_type")

	switch grantType {
	case "authorization_code":
		s.handleAuthorizationCodeGrant(w, r)
	case "refresh_token":
		s.handleRefreshTokenGrant(w, r)
	case "client_credentials":
		s.handleClientCredentialsGrant(w, r)
	case "password":
		s.handlePasswordGrant(w, r)
	default:
		writeError(w, http.StatusBadRequest, "unsupported_grant_type", "Unsupported grant type")
	}
}

// HandleAuthorize handles GET /oauth/authorize
// This implements the OAuth2 authorization code flow
func (s *AuthService) HandleAuthorize(w http.ResponseWriter, r *http.Request) {
	responseType := r.URL.Query().Get("response_type")
	clientID := r.URL.Query().Get("client_id")
	redirectURI := r.URL.Query().Get("redirect_uri")
	scope := r.URL.Query().Get("scope")
	state := r.URL.Query().Get("state")
	codeChallenge := r.URL.Query().Get("code_challenge")
	codeChallengeMethod := r.URL.Query().Get("code_challenge_method")
	nonce := r.URL.Query().Get("nonce")

	// Default code challenge method to S256
	if codeChallengeMethod == "" {
		codeChallengeMethod = "S256"
	}

	// Validate response_type
	if responseType != "code" {
		s.authorizeErrorRedirect(w, redirectURI, "unsupported_response_type", "Only 'code' response type is supported", state)
		return
	}

	// Validate client_id
	if clientID == "" {
		s.authorizeErrorRedirect(w, redirectURI, "invalid_request", "client_id is required", state)
		return
	}

	// Find OAuth client
	oauthClient, err := s.oidcRepo.FindClientByClientID(r.Context(), clientID)
	if err != nil {
		if errors.Is(err, oidc.ErrNotFound) {
			slog.Warn("Authorization request with unknown client_id", "clientId", clientID)
			s.authorizeErrorRedirect(w, redirectURI, "invalid_client", "Unknown client_id", state)
			return
		}
		slog.Error("Failed to find OAuth client", "error", err, "clientId", clientID)
		s.authorizeErrorRedirect(w, redirectURI, "server_error", "Internal server error", state)
		return
	}

	// Check if client is active
	if !oauthClient.Active {
		slog.Warn("Authorization request for inactive client", "clientId", clientID)
		s.authorizeErrorRedirect(w, redirectURI, "invalid_client", "Client is disabled", state)
		return
	}

	// Validate redirect_uri - return error directly (don't redirect to untrusted URI)
	if redirectURI == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "redirect_uri is required")
		return
	}

	if !oauthClient.HasRedirectURI(redirectURI) {
		slog.Warn("Authorization request with invalid redirect_uri", "clientId", clientID, "redirectUri", redirectURI)
		writeError(w, http.StatusBadRequest, "invalid_request", "redirect_uri not allowed for this client")
		return
	}

	// Validate PKCE for public clients or when required
	if oauthClient.IsPublic() || oauthClient.PKCERequired || s.pkceService.IsRequired() {
		if codeChallenge == "" {
			s.authorizeErrorRedirect(w, redirectURI, "invalid_request", "code_challenge required for this client", state)
			return
		}
		if err := s.pkceService.ValidateCodeChallenge(codeChallenge); err != nil {
			s.authorizeErrorRedirect(w, redirectURI, "invalid_request", "Invalid code_challenge format", state)
			return
		}
	}

	// Check if user is authenticated
	sessionToken := s.sessionManager.GetSession(r)
	if sessionToken == "" {
		s.redirectToLogin(w, r, responseType, clientID, redirectURI, scope, state, codeChallenge, codeChallengeMethod, nonce)
		return
	}

	// Validate session and get principal ID
	principalID, err := s.tokenService.ValidateSessionToken(sessionToken)
	if err != nil {
		slog.Debug("Invalid session token in authorize request", "error", err)
		s.redirectToLogin(w, r, responseType, clientID, redirectURI, scope, state, codeChallenge, codeChallengeMethod, nonce)
		return
	}

	// Verify principal exists and is active
	p, err := s.principalRepo.FindByID(r.Context(), principalID)
	if err != nil {
		slog.Debug("Principal not found in authorize request", "error", err, "principalId", principalID)
		s.redirectToLogin(w, r, responseType, clientID, redirectURI, scope, state, codeChallenge, codeChallengeMethod, nonce)
		return
	}

	if !p.Active {
		slog.Warn("Inactive principal in authorize request", "principalId", principalID)
		s.redirectToLogin(w, r, responseType, clientID, redirectURI, scope, state, codeChallenge, codeChallengeMethod, nonce)
		return
	}

	// Check application access restrictions (matching Java's AuthorizationResource)
	if oauthClient.HasApplicationRestrictions() {
		hasAccess := s.checkUserApplicationAccess(p, oauthClient)
		if !hasAccess {
			slog.Warn("User denied access - no application access", "principalId", principalID, "clientId", clientID, "requiredApps", oauthClient.ApplicationIDs)
			s.authorizeErrorRedirect(w, redirectURI, "access_denied", "You don't have access to this application", state)
			return
		}
	}

	// Generate authorization code
	code, err := generateAuthorizationCode()
	if err != nil {
		slog.Error("Failed to generate authorization code", "error", err)
		s.authorizeErrorRedirect(w, redirectURI, "server_error", "Failed to generate authorization code", state)
		return
	}

	// Store authorization code
	authCode := &oidc.AuthorizationCode{
		Code:                code,
		ClientID:            clientID,
		PrincipalID:         principalID,
		RedirectURI:         redirectURI,
		Scope:               scope,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		Nonce:               nonce,
		State:               state,
		ContextClientID:     p.ClientID,
		ExpiresAt:           time.Now().Add(10 * time.Minute),
	}

	if err := s.oidcRepo.SaveAuthorizationCode(r.Context(), authCode); err != nil {
		slog.Error("Failed to save authorization code", "error", err)
		s.authorizeErrorRedirect(w, redirectURI, "server_error", "Failed to create authorization code", state)
		return
	}

	// Build redirect URL with code
	callbackURL := redirectURI
	if strings.Contains(redirectURI, "?") {
		callbackURL += "&"
	} else {
		callbackURL += "?"
	}
	callbackURL += "code=" + url.QueryEscape(code)
	if state != "" {
		callbackURL += "&state=" + url.QueryEscape(state)
	}

	slog.Info("Authorization code issued", "clientId", clientID, "principalId", principalID)

	http.Redirect(w, r, callbackURL, http.StatusFound)
}

// redirectToLogin redirects to the login page preserving OAuth parameters
func (s *AuthService) redirectToLogin(w http.ResponseWriter, r *http.Request, responseType, clientID, redirectURI, scope, state, codeChallenge, codeChallengeMethod, nonce string) {
	loginURL := "/auth/login?oauth=true"
	loginURL += "&response_type=" + url.QueryEscape(responseType)
	loginURL += "&client_id=" + url.QueryEscape(clientID)
	loginURL += "&redirect_uri=" + url.QueryEscape(redirectURI)
	if scope != "" {
		loginURL += "&scope=" + url.QueryEscape(scope)
	}
	if state != "" {
		loginURL += "&state=" + url.QueryEscape(state)
	}
	if codeChallenge != "" {
		loginURL += "&code_challenge=" + url.QueryEscape(codeChallenge)
	}
	if codeChallengeMethod != "" {
		loginURL += "&code_challenge_method=" + url.QueryEscape(codeChallengeMethod)
	}
	if nonce != "" {
		loginURL += "&nonce=" + url.QueryEscape(nonce)
	}

	http.Redirect(w, r, loginURL, http.StatusFound)
}

// authorizeErrorRedirect redirects to the client with an error
func (s *AuthService) authorizeErrorRedirect(w http.ResponseWriter, redirectURI, errCode, errDesc, state string) {
	// If no redirect URI, return error directly
	if redirectURI == "" {
		writeError(w, http.StatusBadRequest, errCode, errDesc)
		return
	}

	errorURL := redirectURI
	if strings.Contains(redirectURI, "?") {
		errorURL += "&"
	} else {
		errorURL += "?"
	}
	errorURL += "error=" + url.QueryEscape(errCode)
	errorURL += "&error_description=" + url.QueryEscape(errDesc)
	if state != "" {
		errorURL += "&state=" + url.QueryEscape(state)
	}

	w.Header().Set("Location", errorURL)
	w.WriteHeader(http.StatusFound)
}

// generateAuthorizationCode generates a secure random authorization code
func generateAuthorizationCode() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func (s *AuthService) handleAuthorizationCodeGrant(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	redirectURI := r.FormValue("redirect_uri")
	clientID := r.FormValue("client_id")
	codeVerifier := r.FormValue("code_verifier")

	if code == "" || redirectURI == "" || clientID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "Missing required parameters")
		return
	}

	// Use the authorization code (single-use)
	authCode, err := s.oidcRepo.UseAuthorizationCode(r.Context(), code)
	if err != nil {
		if errors.Is(err, oidc.ErrNotFound) || errors.Is(err, oidc.ErrAlreadyUsed) {
			writeError(w, http.StatusBadRequest, "invalid_grant", "Invalid or expired authorization code")
			return
		}
		slog.Error("Failed to use authorization code", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "Internal server error")
		return
	}

	// Validate redirect URI matches
	if authCode.RedirectURI != redirectURI {
		writeError(w, http.StatusBadRequest, "invalid_grant", "Redirect URI mismatch")
		return
	}

	// Validate client ID matches
	if authCode.ClientID != clientID {
		writeError(w, http.StatusBadRequest, "invalid_grant", "Client ID mismatch")
		return
	}

	// Verify PKCE if challenge was provided
	if authCode.HasPKCE() {
		if codeVerifier == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "Code verifier required")
			return
		}
		if err := s.pkceService.VerifyCodeChallenge(codeVerifier, authCode.CodeChallenge, authCode.CodeChallengeMethod); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_grant", "PKCE verification failed")
			return
		}
	}

	// Get the principal
	p, err := s.principalRepo.FindByID(r.Context(), authCode.PrincipalID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_grant", "User not found")
		return
	}

	// Get client IDs for token
	clients := s.getClientIDsForPrincipal(r.Context(), p)
	applications := extractApplicationCodes(p.GetRoleNames())

	email := ""
	name := ""
	if p.UserIdentity != nil {
		email = p.UserIdentity.Email
		name = p.Name
	}

	// Issue access token
	accessToken, err := s.tokenService.IssueSessionToken(
		p.ID,
		email,
		p.GetRoleNames(),
		clients,
		applications,
	)
	if err != nil {
		slog.Error("Failed to issue access token", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "Failed to issue tokens")
		return
	}

	// Generate refresh token
	refreshTokenRaw, err := jwt.GenerateRefreshToken()
	if err != nil {
		slog.Error("Failed to generate refresh token", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "Failed to issue tokens")
		return
	}

	// Store refresh token
	refreshToken := &oidc.RefreshToken{
		TokenHash:       jwt.HashToken(refreshTokenRaw),
		PrincipalID:     p.ID,
		ClientID:        clientID,
		ContextClientID: authCode.ContextClientID,
		Scope:           authCode.Scope,
		TokenFamily:     tsid.Generate(),
		ExpiresAt:       time.Now().Add(30 * 24 * time.Hour),
	}
	if err := s.oidcRepo.SaveRefreshToken(r.Context(), refreshToken); err != nil {
		slog.Error("Failed to save refresh token", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "Failed to issue tokens")
		return
	}

	resp := TokenResponse{
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		ExpiresIn:    3600, // 1 hour
		RefreshToken: refreshTokenRaw,
		Scope:        authCode.Scope,
	}

	// Issue ID token if openid scope was requested
	if strings.Contains(authCode.Scope, "openid") {
		idToken, err := s.tokenService.IssueIDToken(
			p.ID,
			email,
			name,
			clientID,
			authCode.Nonce,
			clients,
		)
		if err == nil {
			resp.IDToken = idToken
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *AuthService) handleRefreshTokenGrant(w http.ResponseWriter, r *http.Request) {
	refreshTokenRaw := r.FormValue("refresh_token")
	if refreshTokenRaw == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "Refresh token required")
		return
	}

	// Find the refresh token
	refreshToken, err := s.oidcRepo.FindRefreshTokenByRaw(r.Context(), refreshTokenRaw)
	if err != nil {
		if errors.Is(err, oidc.ErrNotFound) {
			writeError(w, http.StatusBadRequest, "invalid_grant", "Invalid refresh token")
			return
		}
		slog.Error("Failed to find refresh token", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "Internal server error")
		return
	}

	// Check if revoked (potential token reuse attack)
	if refreshToken.Revoked {
		// Revoke entire token family
		_ = s.oidcRepo.RevokeTokenFamily(r.Context(), refreshToken.TokenFamily)
		writeError(w, http.StatusBadRequest, "invalid_grant", "Token has been revoked")
		return
	}

	// Check expiration
	if refreshToken.IsExpired() {
		writeError(w, http.StatusBadRequest, "invalid_grant", "Refresh token expired")
		return
	}

	// Get the principal
	p, err := s.principalRepo.FindByID(r.Context(), refreshToken.PrincipalID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_grant", "User not found")
		return
	}

	if !p.Active {
		writeError(w, http.StatusBadRequest, "invalid_grant", "Account is disabled")
		return
	}

	// Get client IDs for token
	clients := s.getClientIDsForPrincipal(r.Context(), p)
	applications := extractApplicationCodes(p.GetRoleNames())

	email := ""
	if p.UserIdentity != nil {
		email = p.UserIdentity.Email
	}

	// Issue new access token
	accessToken, err := s.tokenService.IssueSessionToken(
		p.ID,
		email,
		p.GetRoleNames(),
		clients,
		applications,
	)
	if err != nil {
		slog.Error("Failed to issue access token", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "Failed to issue tokens")
		return
	}

	// Generate new refresh token (rotation)
	newRefreshTokenRaw, err := jwt.GenerateRefreshToken()
	if err != nil {
		slog.Error("Failed to generate refresh token", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "Failed to issue tokens")
		return
	}

	// Revoke old token and save new one
	newTokenHash := jwt.HashToken(newRefreshTokenRaw)
	_ = s.oidcRepo.RevokeRefreshToken(r.Context(), refreshToken.TokenHash, newTokenHash)

	newRefreshToken := &oidc.RefreshToken{
		TokenHash:       newTokenHash,
		PrincipalID:     p.ID,
		ClientID:        refreshToken.ClientID,
		ContextClientID: refreshToken.ContextClientID,
		Scope:           refreshToken.Scope,
		TokenFamily:     refreshToken.TokenFamily, // Same family for rotation tracking
		ExpiresAt:       time.Now().Add(30 * 24 * time.Hour),
	}
	if err := s.oidcRepo.SaveRefreshToken(r.Context(), newRefreshToken); err != nil {
		slog.Error("Failed to save refresh token", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "Failed to issue tokens")
		return
	}

	resp := TokenResponse{
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		RefreshToken: newRefreshTokenRaw,
		Scope:        refreshToken.Scope,
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *AuthService) handleClientCredentialsGrant(w http.ResponseWriter, r *http.Request) {
	clientID, clientSecret, ok := r.BasicAuth()
	if !ok {
		clientID = r.FormValue("client_id")
		clientSecret = r.FormValue("client_secret")
	}

	if clientID == "" || clientSecret == "" {
		writeError(w, http.StatusUnauthorized, "invalid_client", "Client credentials required")
		return
	}

	// Find OAuth client
	oauthClient, err := s.oidcRepo.FindClientByClientID(r.Context(), clientID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_client", "Invalid client")
		return
	}

	if !oauthClient.Active {
		writeError(w, http.StatusUnauthorized, "invalid_client", "Client is disabled")
		return
	}

	if !oauthClient.HasGrantType("client_credentials") {
		writeError(w, http.StatusUnauthorized, "unauthorized_client", "Client not authorized for this grant type")
		return
	}

	// TODO: Verify client secret using secret provider
	// For now, we'll skip secret verification as it requires the secret provider

	// Get the service account principal
	if oauthClient.ServiceAccountPrincipalID == "" {
		writeError(w, http.StatusUnauthorized, "invalid_client", "No service account configured")
		return
	}

	p, err := s.principalRepo.FindByID(r.Context(), oauthClient.ServiceAccountPrincipalID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_client", "Service account not found")
		return
	}

	// Issue access token
	accessToken, err := s.tokenService.IssueAccessToken(
		p.ID,
		p.ClientID,
		p.GetRoleNames(),
	)
	if err != nil {
		slog.Error("Failed to issue access token", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "Failed to issue token")
		return
	}

	resp := TokenResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   3600,
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *AuthService) handlePasswordGrant(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == "" || password == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "Username and password required")
		return
	}

	// Normalize email
	email := local.NormalizeEmail(username)

	// Find principal by email
	p, err := s.principalRepo.FindByEmail(r.Context(), email)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_grant", "Invalid credentials")
		return
	}

	if !p.Active {
		writeError(w, http.StatusUnauthorized, "invalid_grant", "Account is disabled")
		return
	}

	if p.UserIdentity == nil || p.UserIdentity.IdpType != principal.IdpTypeInternal {
		writeError(w, http.StatusUnauthorized, "invalid_grant", "Password authentication not available")
		return
	}

	// Verify password
	if err := s.passwordService.VerifyPassword(password, p.UserIdentity.PasswordHash); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_grant", "Invalid credentials")
		return
	}

	// Update last login
	_ = s.principalRepo.UpdateLastLogin(r.Context(), p.ID)

	// Get client IDs for token
	clients := s.getClientIDsForPrincipal(r.Context(), p)
	applications := extractApplicationCodes(p.GetRoleNames())

	// Issue access token
	accessToken, err := s.tokenService.IssueSessionToken(
		p.ID,
		p.UserIdentity.Email,
		p.GetRoleNames(),
		clients,
		applications,
	)
	if err != nil {
		slog.Error("Failed to issue access token", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "Failed to issue token")
		return
	}

	// Generate refresh token
	refreshTokenRaw, err := jwt.GenerateRefreshToken()
	if err != nil {
		slog.Error("Failed to generate refresh token", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "Failed to issue tokens")
		return
	}

	// Store refresh token
	refreshToken := &oidc.RefreshToken{
		TokenHash:   jwt.HashToken(refreshTokenRaw),
		PrincipalID: p.ID,
		TokenFamily: tsid.Generate(),
		ExpiresAt:   time.Now().Add(30 * 24 * time.Hour),
	}
	if err := s.oidcRepo.SaveRefreshToken(r.Context(), refreshToken); err != nil {
		slog.Error("Failed to save refresh token", "error", err)
		// Continue without refresh token
	}

	resp := TokenResponse{
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		RefreshToken: refreshTokenRaw,
	}

	writeJSON(w, http.StatusOK, resp)
}

// === OIDC Federation Handlers ===

// HandleOIDCLogin handles GET /auth/oidc/login
// Initiates the federated login flow with an upstream IDP
func (s *AuthService) HandleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")
	if domain == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "Domain is required")
		return
	}

	// Find the auth config for this domain
	authConfig, err := s.clientRepo.FindAuthConfigByDomain(r.Context(), domain)
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "No auth configuration for domain")
			return
		}
		slog.Error("Failed to find auth config", "error", err, "domain", domain)
		writeError(w, http.StatusInternalServerError, "server_error", "Internal server error")
		return
	}

	if !authConfig.IsOIDC() {
		writeError(w, http.StatusBadRequest, "invalid_request", "Domain does not use OIDC authentication")
		return
	}

	// Check if we have an adapter for this domain
	if !s.federationService.HasAdapter(domain) {
		// Try to create one from the config
		idpType := federation.IdpTypeOIDC
		if authConfig.IdpType == "KEYCLOAK" {
			idpType = federation.IdpTypeKeycloak
		} else if authConfig.IdpType == "ENTRA" || authConfig.IdpType == "AZURE_AD" {
			idpType = federation.IdpTypeEntra
		}

		federationConfig := &federation.Config{
			IssuerURL:    authConfig.OIDCIssuerURL,
			ClientID:     authConfig.OIDCClientID,
			ClientSecret: authConfig.OIDCClientSecret,
			TenantID:     authConfig.EntraTenantID,
			Scopes:       []string{"openid", "profile", "email"},
			GroupsClaim:  authConfig.GroupsClaim,
			RolesClaim:   authConfig.RolesClaim,
		}

		if err := s.federationService.CreateAdapter(domain, idpType, federationConfig); err != nil {
			slog.Error("Failed to create federation adapter", "error", err, "domain", domain)
			writeError(w, http.StatusInternalServerError, "server_error", "Failed to configure identity provider")
			return
		}
	}

	// Generate PKCE
	codeVerifier, _, err := s.pkceService.GeneratePKCEPair()
	if err != nil {
		slog.Error("Failed to generate PKCE", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "Internal server error")
		return
	}

	// Generate state and nonce
	state, _ := federation.GenerateRandomString(32)
	nonce, _ := federation.GenerateRandomString(32)

	// Store login state
	loginState := &oidc.OIDCLoginState{
		State:        state,
		Nonce:        nonce,
		CodeVerifier: codeVerifier,
		Domain:       domain,
		ClientID:     authConfig.ClientID,
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}
	if err := s.oidcRepo.SaveLoginState(r.Context(), loginState); err != nil {
		slog.Error("Failed to save login state", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "Internal server error")
		return
	}

	// Build redirect URI
	redirectURI := s.externalURL + "/auth/oidc/callback"

	// Get authorization URL
	authReq := &federation.AuthRequest{
		ClientID:     authConfig.OIDCClientID,
		RedirectURI:  redirectURI,
		State:        state,
		Nonce:        nonce,
		CodeVerifier: codeVerifier,
	}

	authURL, err := s.federationService.InitiateLogin(r.Context(), domain, authReq)
	if err != nil {
		slog.Error("Failed to initiate login", "error", err, "domain", domain)
		writeError(w, http.StatusInternalServerError, "server_error", "Failed to initiate login")
		return
	}

	// Redirect to IDP
	http.Redirect(w, r, authURL, http.StatusFound)
}

// HandleOIDCCallback handles GET /auth/oidc/callback
// Processes the callback from an upstream IDP
func (s *AuthService) HandleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	// Check for error from IDP
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		errDesc := r.URL.Query().Get("error_description")
		slog.Warn("OIDC callback error", "error", errParam, "description", errDesc)
		s.redirectWithError(w, r, errParam, errDesc)
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "Missing code or state")
		return
	}

	// Consume login state (single-use)
	loginState, err := s.oidcRepo.ConsumeLoginState(r.Context(), state)
	if err != nil {
		if errors.Is(err, oidc.ErrNotFound) {
			writeError(w, http.StatusBadRequest, "invalid_state", "Invalid or expired state")
			return
		}
		if errors.Is(err, oidc.ErrExpired) {
			writeError(w, http.StatusBadRequest, "invalid_state", "State has expired")
			return
		}
		slog.Error("Failed to consume login state", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "Internal server error")
		return
	}

	// Build redirect URI (must match the one used in login)
	redirectURI := s.externalURL + "/auth/oidc/callback"

	// Exchange code for tokens and get user info
	userInfo, _, err := s.federationService.HandleCallback(
		r.Context(),
		loginState.Domain,
		code,
		redirectURI,
		loginState.CodeVerifier,
		loginState.Nonce,
	)
	if err != nil {
		slog.Error("Failed to handle OIDC callback", "error", err, "domain", loginState.Domain)
		s.redirectWithError(w, r, "authentication_failed", "Failed to authenticate with identity provider")
		return
	}

	// Find or create principal
	p, err := s.findOrCreateFederatedPrincipal(r.Context(), loginState, userInfo)
	if err != nil {
		slog.Error("Failed to find or create principal", "error", err, "email", userInfo.Email)
		s.redirectWithError(w, r, "provisioning_failed", "Failed to provision user account")
		return
	}

	// Check if principal is active
	if !p.Active {
		s.redirectWithError(w, r, "account_disabled", "Account is disabled")
		return
	}

	// Update last login
	_ = s.principalRepo.UpdateLastLogin(r.Context(), p.ID)

	// Get client IDs for token
	clients := s.getClientIDsForPrincipal(r.Context(), p)
	applications := extractApplicationCodes(p.GetRoleNames())

	email := ""
	if p.UserIdentity != nil {
		email = p.UserIdentity.Email
	}

	// Issue session token
	token, err := s.tokenService.IssueSessionToken(
		p.ID,
		email,
		p.GetRoleNames(),
		clients,
		applications,
	)
	if err != nil {
		slog.Error("Failed to issue session token", "error", err)
		s.redirectWithError(w, r, "token_error", "Failed to create session")
		return
	}

	// Set session cookie
	s.sessionManager.SetSession(w, token)

	// Redirect to success page
	successURL := s.externalURL + "/"
	http.Redirect(w, r, successURL, http.StatusFound)
}

// findOrCreateFederatedPrincipal finds or creates a principal for a federated user
func (s *AuthService) findOrCreateFederatedPrincipal(ctx context.Context, loginState *oidc.OIDCLoginState, userInfo *federation.UserInfo) (*principal.Principal, error) {
	if userInfo.Email == "" {
		return nil, errors.New("email is required from identity provider")
	}

	email := local.NormalizeEmail(userInfo.Email)

	// Try to find existing principal
	p, err := s.principalRepo.FindByEmail(ctx, email)
	if err == nil {
		// Found existing principal - update identity info if needed
		if p.UserIdentity != nil {
			p.UserIdentity.IdpSubject = userInfo.Subject
			p.UserIdentity.EmailVerified = userInfo.EmailVerified
			_ = s.principalRepo.Update(ctx, p)
		}
		return p, nil
	}

	if !errors.Is(err, principal.ErrNotFound) {
		return nil, err
	}

	// Create new principal
	name := userInfo.Name
	if name == "" {
		name = userInfo.GivenName
		if userInfo.FamilyName != "" {
			if name != "" {
				name += " "
			}
			name += userInfo.FamilyName
		}
	}
	if name == "" {
		name = email
	}

	// Determine scope and client ID based on domain configuration
	scope := principal.UserScopeClient
	clientID := loginState.ClientID

	p = &principal.Principal{
		ID:     tsid.Generate(),
		Name:   name,
		Type:   principal.PrincipalTypeUser,
		Active: true,
		Scope:  scope,
		UserIdentity: &principal.UserIdentity{
			Email:         email,
			EmailVerified: userInfo.EmailVerified,
			IdpType:       principal.IdpTypeExternal,
			IdpIssuer:     "", // Will be set from userInfo if available
			IdpSubject:    userInfo.Subject,
		},
		ClientID: clientID,
	}

	// Map IDP groups/roles to FlowCatalyst roles
	p.Roles = s.mapIdpRolesToPrincipalRoles(ctx, loginState.Domain, userInfo)

	if err := s.principalRepo.Insert(ctx, p); err != nil {
		return nil, err
	}

	slog.Info("Created federated user", "principalId", p.ID, "email", email, "domain", loginState.Domain)

	return p, nil
}

// mapIdpRolesToPrincipalRoles maps IDP groups/roles to FlowCatalyst roles
func (s *AuthService) mapIdpRolesToPrincipalRoles(ctx context.Context, domain string, userInfo *federation.UserInfo) []principal.RoleAssignment {
	// Get role mappings for this domain
	mappings, err := s.clientRepo.FindIdpRoleMappingsByDomain(ctx, domain)
	if err != nil || len(mappings) == 0 {
		return nil
	}

	roleSet := make(map[string]bool)

	// Check groups
	for _, group := range userInfo.Groups {
		for _, mapping := range mappings {
			if mapping.IdpGroupName == group {
				roleSet[mapping.RoleID] = true
			}
		}
	}

	// Check roles from IDP
	for _, role := range userInfo.Roles {
		for _, mapping := range mappings {
			if mapping.IdpGroupName == role {
				roleSet[mapping.RoleID] = true
			}
		}
	}

	// Convert to role assignments
	var roles []principal.RoleAssignment
	for roleID := range roleSet {
		roles = append(roles, principal.RoleAssignment{
			RoleID:     roleID,
			AssignedAt: time.Now(),
		})
	}

	return roles
}

// redirectWithError redirects to the frontend with an error
func (s *AuthService) redirectWithError(w http.ResponseWriter, r *http.Request, errCode, errDesc string) {
	errorURL := s.externalURL + "/auth/error?error=" + errCode + "&error_description=" + errDesc
	http.Redirect(w, r, errorURL, http.StatusFound)
}
