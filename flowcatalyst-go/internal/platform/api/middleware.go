package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"go.flowcatalyst.tech/internal/platform/auth/jwt"
	"go.flowcatalyst.tech/internal/platform/auth/session"
	"go.flowcatalyst.tech/internal/platform/principal"
)

// ContextKey is a type for context keys
type ContextKey string

const (
	// ContextKeyPrincipal is the key for the authenticated principal
	ContextKeyPrincipal ContextKey = "principal"
	// ContextKeyClaims is the key for the JWT claims
	ContextKeyClaims ContextKey = "claims"
	// ContextKeyClientID is the key for the current client context
	ContextKeyClientID ContextKey = "clientId"
)

// AuthMiddleware provides authentication and authorization middleware
type AuthMiddleware struct {
	tokenService   *jwt.TokenService
	sessionManager *session.Manager
	principalRepo  principal.Repository
}

// NewAuthMiddleware creates a new auth middleware
func NewAuthMiddleware(
	tokenService *jwt.TokenService,
	sessionManager *session.Manager,
	principalRepo principal.Repository,
) *AuthMiddleware {
	return &AuthMiddleware{
		tokenService:   tokenService,
		sessionManager: sessionManager,
		principalRepo:  principalRepo,
	}
}

// RequireAuth ensures the request has a valid authentication token
func (m *AuthMiddleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to get token from Authorization header first
		token := extractBearerToken(r)

		// Fall back to session cookie
		if token == "" {
			token = m.sessionManager.GetSession(r)
		}

		if token == "" {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
			return
		}

		// Validate token
		principalID, err := m.tokenService.ValidateSessionToken(token)
		if err != nil {
			slog.Debug("Token validation failed", "error", err)
			writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Invalid or expired token")
			return
		}

		// Load principal
		p, err := m.principalRepo.FindByID(r.Context(), principalID)
		if err != nil {
			slog.Debug("Principal not found", "error", err, "principalId", principalID)
			writeJSONError(w, http.StatusUnauthorized, "unauthorized", "User not found")
			return
		}

		if !p.Active {
			writeJSONError(w, http.StatusForbidden, "forbidden", "Account is disabled")
			return
		}

		// Add principal to context
		ctx := context.WithValue(r.Context(), ContextKeyPrincipal, p)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRole ensures the principal has one of the specified roles
func (m *AuthMiddleware) RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := GetPrincipal(r.Context())
			if p == nil {
				writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
				return
			}

			// Check if principal has any of the required roles
			for _, role := range roles {
				if p.HasRole(role) {
					next.ServeHTTP(w, r)
					return
				}
			}

			writeJSONError(w, http.StatusForbidden, "forbidden", "Insufficient permissions")
		})
	}
}

// RequireAnchor ensures the principal has anchor (platform admin) scope
func (m *AuthMiddleware) RequireAnchor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := GetPrincipal(r.Context())
		if p == nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
			return
		}

		if !p.IsAnchor() {
			writeJSONError(w, http.StatusForbidden, "forbidden", "Platform admin access required")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RequireClientAccess ensures the principal has access to the specified client
func (m *AuthMiddleware) RequireClientAccess(clientIDParam string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := GetPrincipal(r.Context())
			if p == nil {
				writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
				return
			}

			// Anchor users have access to all clients
			if p.IsAnchor() {
				next.ServeHTTP(w, r)
				return
			}

			// Get client ID from URL or header
			clientID := r.PathValue(clientIDParam)
			if clientID == "" {
				clientID = r.Header.Get("X-Client-ID")
			}

			if clientID == "" {
				writeJSONError(w, http.StatusBadRequest, "bad_request", "Client ID required")
				return
			}

			// Check if principal has access to this client
			if !hasClientAccess(p, clientID) {
				writeJSONError(w, http.StatusForbidden, "forbidden", "No access to this client")
				return
			}

			// Add client ID to context
			ctx := context.WithValue(r.Context(), ContextKeyClientID, clientID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalAuth tries to authenticate but allows unauthenticated requests
func (m *AuthMiddleware) OptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			token = m.sessionManager.GetSession(r)
		}

		if token != "" {
			principalID, err := m.tokenService.ValidateSessionToken(token)
			if err == nil {
				p, err := m.principalRepo.FindByID(r.Context(), principalID)
				if err == nil && p.Active {
					ctx := context.WithValue(r.Context(), ContextKeyPrincipal, p)
					r = r.WithContext(ctx)
				}
			}
		}

		next.ServeHTTP(w, r)
	})
}

// === Helper functions ===

// GetPrincipal returns the authenticated principal from the context
func GetPrincipal(ctx context.Context) *principal.Principal {
	p, _ := ctx.Value(ContextKeyPrincipal).(*principal.Principal)
	return p
}

// GetClientID returns the current client ID from the context
func GetClientID(ctx context.Context) string {
	id, _ := ctx.Value(ContextKeyClientID).(string)
	return id
}

// extractBearerToken extracts the token from the Authorization header
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}

	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}

	return parts[1]
}

// hasClientAccess checks if a principal has access to a client
func hasClientAccess(p *principal.Principal, clientID string) bool {
	// Check home client
	if p.ClientID == clientID {
		return true
	}

	// Partner users might have access through grants
	// This would need to be checked against the access grants
	// For now, we only check the home client

	return false
}

// writeJSONError writes a JSON error response
func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write([]byte(`{"error":"` + code + `","message":"` + message + `"}`))
}
