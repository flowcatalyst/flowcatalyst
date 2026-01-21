package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// === Request/Response DTO Tests ===

func TestLoginRequest_JSON(t *testing.T) {
	jsonData := `{"email": "test@example.com", "password": "secret123"}`

	var req LoginRequest
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if req.Email != "test@example.com" {
		t.Errorf("Expected email 'test@example.com', got '%s'", req.Email)
	}
	if req.Password != "secret123" {
		t.Errorf("Expected password 'secret123', got '%s'", req.Password)
	}
}

func TestLoginResponse_JSON(t *testing.T) {
	resp := LoginResponse{
		PrincipalID: "principal-123",
		Name:        "John Doe",
		Email:       "john@example.com",
		Roles:       []string{"admin", "user"},
		ClientID:    "client-456",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	jsonStr := string(data)

	// Verify camelCase field names
	expectedFields := []string{
		`"principalId"`, `"name"`, `"email"`, `"roles"`, `"clientId"`,
	}
	for _, field := range expectedFields {
		if !strings.Contains(jsonStr, field) {
			t.Errorf("Expected %s in JSON, got %s", field, jsonStr)
		}
	}
}

func TestLoginResponse_OmitEmptyClientID(t *testing.T) {
	resp := LoginResponse{
		PrincipalID: "principal-123",
		Name:        "John Doe",
		Email:       "john@example.com",
		Roles:       []string{"user"},
		ClientID:    "", // Empty
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// clientId should be omitted when empty
	if strings.Contains(string(data), `"clientId"`) {
		t.Error("Expected clientId to be omitted when empty")
	}
}

func TestDomainCheckRequest_JSON(t *testing.T) {
	jsonData := `{"email": "user@acme.com"}`

	var req DomainCheckRequest
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if req.Email != "user@acme.com" {
		t.Errorf("Expected email 'user@acme.com', got '%s'", req.Email)
	}
}

func TestDomainCheckResponse_Internal(t *testing.T) {
	resp := DomainCheckResponse{
		AuthMethod: "internal",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	if !strings.Contains(string(data), `"authMethod":"internal"`) {
		t.Errorf("Expected authMethod 'internal', got %s", string(data))
	}

	// loginUrl and idpIssuer should be omitted
	if strings.Contains(string(data), `"loginUrl"`) {
		t.Error("Expected loginUrl to be omitted for internal auth")
	}
}

func TestDomainCheckResponse_External(t *testing.T) {
	resp := DomainCheckResponse{
		AuthMethod: "external",
		LoginURL:   "/auth/oidc/login?domain=acme.com",
		IdpIssuer:  "https://login.microsoftonline.com/tenant/v2.0",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"authMethod":"external"`) {
		t.Errorf("Expected authMethod 'external', got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"loginUrl"`) {
		t.Error("Expected loginUrl to be present for external auth")
	}
	if !strings.Contains(jsonStr, `"idpIssuer"`) {
		t.Error("Expected idpIssuer to be present for external auth")
	}
}

func TestMessageResponse_JSON(t *testing.T) {
	resp := MessageResponse{Message: "Logged out successfully"}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	if !strings.Contains(string(data), `"message":"Logged out successfully"`) {
		t.Errorf("Unexpected JSON: %s", string(data))
	}
}

func TestErrorResponse_JSON(t *testing.T) {
	resp := ErrorResponse{
		Error:            "invalid_credentials",
		ErrorDescription: "Invalid email or password",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"error":"invalid_credentials"`) {
		t.Errorf("Expected error field, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"error_description":"Invalid email or password"`) {
		t.Errorf("Expected error_description field, got %s", jsonStr)
	}
}

func TestErrorResponse_OmitEmptyDescription(t *testing.T) {
	resp := ErrorResponse{
		Error: "server_error",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	if strings.Contains(string(data), `"error_description"`) {
		t.Error("Expected error_description to be omitted when empty")
	}
}

// === Token Request/Response Tests ===

func TestTokenRequest_JSON(t *testing.T) {
	jsonData := `{
		"grant_type": "authorization_code",
		"code": "abc123",
		"redirect_uri": "http://localhost:3000/callback",
		"client_id": "my-client",
		"code_verifier": "verifier123"
	}`

	var req TokenRequest
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if req.GrantType != "authorization_code" {
		t.Errorf("Expected grant_type 'authorization_code', got '%s'", req.GrantType)
	}
	if req.Code != "abc123" {
		t.Errorf("Expected code 'abc123', got '%s'", req.Code)
	}
	if req.RedirectURI != "http://localhost:3000/callback" {
		t.Errorf("Expected redirect_uri 'http://localhost:3000/callback', got '%s'", req.RedirectURI)
	}
}

func TestTokenResponse_JSON(t *testing.T) {
	resp := TokenResponse{
		AccessToken:  "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		RefreshToken: "refresh-token-123",
		Scope:        "openid profile email",
		IDToken:      "id-token-123",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	jsonStr := string(data)
	expectedFields := []string{
		`"access_token"`, `"token_type"`, `"expires_in"`,
		`"refresh_token"`, `"scope"`, `"id_token"`,
	}
	for _, field := range expectedFields {
		if !strings.Contains(jsonStr, field) {
			t.Errorf("Expected %s in JSON, got %s", field, jsonStr)
		}
	}
}

func TestTokenResponse_MinimalFields(t *testing.T) {
	resp := TokenResponse{
		AccessToken: "access-token",
		TokenType:   "Bearer",
		ExpiresIn:   3600,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	jsonStr := string(data)

	// Required fields should be present
	if !strings.Contains(jsonStr, `"access_token"`) {
		t.Error("Expected access_token")
	}
	if !strings.Contains(jsonStr, `"token_type"`) {
		t.Error("Expected token_type")
	}
	if !strings.Contains(jsonStr, `"expires_in"`) {
		t.Error("Expected expires_in")
	}

	// Optional fields should be omitted
	if strings.Contains(jsonStr, `"refresh_token"`) {
		t.Error("Expected refresh_token to be omitted")
	}
	if strings.Contains(jsonStr, `"id_token"`) {
		t.Error("Expected id_token to be omitted")
	}
}

// === Helper Function Tests ===

func TestExtractApplicationCodes(t *testing.T) {
	tests := []struct {
		name     string
		roles    []string
		expected map[string]bool
	}{
		{
			name:     "empty roles",
			roles:    []string{},
			expected: map[string]bool{},
		},
		{
			name:     "single role with prefix",
			roles:    []string{"platform:admin"},
			expected: map[string]bool{"platform": true},
		},
		{
			name:     "multiple roles same app",
			roles:    []string{"platform:admin", "platform:viewer"},
			expected: map[string]bool{"platform": true},
		},
		{
			name:     "multiple apps",
			roles:    []string{"platform:admin", "dispatch:manager", "events:viewer"},
			expected: map[string]bool{"platform": true, "dispatch": true, "events": true},
		},
		{
			name:     "role without prefix",
			roles:    []string{"admin"},
			expected: map[string]bool{"admin": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractApplicationCodes(tt.roles)
			resultMap := make(map[string]bool)
			for _, code := range result {
				resultMap[code] = true
			}

			if len(resultMap) != len(tt.expected) {
				t.Errorf("Expected %d codes, got %d", len(tt.expected), len(resultMap))
			}

			for code := range tt.expected {
				if !resultMap[code] {
					t.Errorf("Expected code '%s' not found", code)
				}
			}
		})
	}
}

// === writeJSON and writeError Tests ===

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"message": "hello"}

	writeJSON(w, http.StatusOK, data)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", w.Header().Get("Content-Type"))
	}

	var result map[string]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result["message"] != "hello" {
		t.Errorf("Expected message 'hello', got '%s'", result["message"])
	}
}

func TestWriteError(t *testing.T) {
	tests := []struct {
		status      int
		err         string
		description string
	}{
		{http.StatusBadRequest, "invalid_request", "Invalid request body"},
		{http.StatusUnauthorized, "invalid_credentials", "Invalid email or password"},
		{http.StatusForbidden, "access_denied", "Access denied"},
		{http.StatusInternalServerError, "server_error", "Internal server error"},
	}

	for _, tt := range tests {
		t.Run(tt.err, func(t *testing.T) {
			w := httptest.NewRecorder()

			writeError(w, tt.status, tt.err, tt.description)

			if w.Code != tt.status {
				t.Errorf("Expected status %d, got %d", tt.status, w.Code)
			}

			var resp ErrorResponse
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if resp.Error != tt.err {
				t.Errorf("Expected error '%s', got '%s'", tt.err, resp.Error)
			}
			if resp.ErrorDescription != tt.description {
				t.Errorf("Expected description '%s', got '%s'", tt.description, resp.ErrorDescription)
			}
		})
	}
}

// === Grant Type Constants ===

func TestGrantTypes(t *testing.T) {
	validGrants := []string{
		"authorization_code",
		"refresh_token",
		"client_credentials",
		"password",
	}

	for _, grant := range validGrants {
		t.Run(grant, func(t *testing.T) {
			// Just verify these are the expected strings
			if grant == "" {
				t.Error("Grant type should not be empty")
			}
		})
	}
}

// === OAuth Error Codes ===

func TestOAuthErrorCodes(t *testing.T) {
	// Standard OAuth 2.0 error codes
	errorCodes := []string{
		"invalid_request",
		"invalid_client",
		"invalid_grant",
		"unauthorized_client",
		"unsupported_grant_type",
		"server_error",
	}

	for _, code := range errorCodes {
		t.Run(code, func(t *testing.T) {
			resp := ErrorResponse{Error: code}
			data, _ := json.Marshal(resp)
			if !strings.Contains(string(data), code) {
				t.Errorf("Error code '%s' not in response", code)
			}
		})
	}
}

// === Token Response Expiration ===

func TestTokenResponseExpiresIn(t *testing.T) {
	tests := []struct {
		name      string
		expiresIn int64
	}{
		{"one hour", 3600},
		{"thirty minutes", 1800},
		{"one day", 86400},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := TokenResponse{
				AccessToken: "token",
				TokenType:   "Bearer",
				ExpiresIn:   tt.expiresIn,
			}

			data, _ := json.Marshal(resp)
			var decoded TokenResponse
			json.Unmarshal(data, &decoded)

			if decoded.ExpiresIn != tt.expiresIn {
				t.Errorf("Expected expires_in %d, got %d", tt.expiresIn, decoded.ExpiresIn)
			}
		})
	}
}

// === Login Response with Roles ===

func TestLoginResponseRoles(t *testing.T) {
	resp := LoginResponse{
		PrincipalID: "p-1",
		Name:        "User",
		Email:       "user@example.com",
		Roles:       []string{"platform:admin", "dispatch:manager"},
	}

	data, _ := json.Marshal(resp)

	var decoded LoginResponse
	json.Unmarshal(data, &decoded)

	if len(decoded.Roles) != 2 {
		t.Errorf("Expected 2 roles, got %d", len(decoded.Roles))
	}

	if decoded.Roles[0] != "platform:admin" {
		t.Errorf("Expected first role 'platform:admin', got '%s'", decoded.Roles[0])
	}
}

// === HTTP Status Codes ===

func TestHTTPStatusCodes(t *testing.T) {
	tests := []struct {
		scenario string
		status   int
	}{
		{"successful login", http.StatusOK},
		{"created resource", http.StatusCreated},
		{"bad request", http.StatusBadRequest},
		{"unauthorized", http.StatusUnauthorized},
		{"forbidden", http.StatusForbidden},
		{"not found", http.StatusNotFound},
		{"conflict", http.StatusConflict},
		{"internal error", http.StatusInternalServerError},
		{"service unavailable", http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.scenario, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeJSON(w, tt.status, map[string]string{"test": "test"})

			if w.Code != tt.status {
				t.Errorf("Expected status %d, got %d", tt.status, w.Code)
			}
		})
	}
}
