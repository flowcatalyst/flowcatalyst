package api

import (
	"encoding/json"
	"testing"
	"time"

	"go.flowcatalyst.tech/internal/platform/client"
)

// Tests for AuthConfigDTO

func TestAuthConfigDTO_JSON(t *testing.T) {
	dto := AuthConfigDTO{
		ID:              "config-123",
		EmailDomain:     "acme.com",
		ConfigType:      "CLIENT",
		PrimaryClientID: "client-456",
		AuthProvider:    "OIDC",
		IdpType:         "ENTRA",
		OIDCIssuerURL:   "https://login.microsoftonline.com/tenant/v2.0",
		OIDCClientID:    "app-id",
		CreatedAt:       "2024-01-01T00:00:00Z",
		UpdatedAt:       "2024-01-01T00:00:00Z",
	}

	data, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("Failed to marshal AuthConfigDTO: %v", err)
	}

	jsonStr := string(data)

	// Verify camelCase field names
	expectedFields := []string{
		`"id"`, `"emailDomain"`, `"configType"`, `"primaryClientId"`,
		`"authProvider"`, `"idpType"`, `"oidcIssuerUrl"`, `"oidcClientId"`,
	}
	for _, field := range expectedFields {
		if !contains(jsonStr, field) {
			t.Errorf("Expected %s in JSON, got %s", field, jsonStr)
		}
	}
}

func TestCreateAuthConfigRequest_JSON(t *testing.T) {
	jsonData := `{
		"emailDomain": "acme.com",
		"configType": "CLIENT",
		"primaryClientId": "client-123",
		"authProvider": "OIDC",
		"idpType": "KEYCLOAK",
		"oidcIssuerUrl": "https://keycloak.example.com/realms/acme",
		"oidcClientId": "flowcatalyst"
	}`

	var req CreateAuthConfigRequest
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if req.EmailDomain != "acme.com" {
		t.Errorf("Expected EmailDomain 'acme.com', got '%s'", req.EmailDomain)
	}
	if req.ConfigType != "CLIENT" {
		t.Errorf("Expected ConfigType 'CLIENT', got '%s'", req.ConfigType)
	}
	if req.PrimaryClientID != "client-123" {
		t.Errorf("Expected PrimaryClientID 'client-123', got '%s'", req.PrimaryClientID)
	}
	if req.AuthProvider != "OIDC" {
		t.Errorf("Expected AuthProvider 'OIDC', got '%s'", req.AuthProvider)
	}
}

func TestToAuthConfigDTO(t *testing.T) {
	now := time.Now()
	config := &client.ClientAuthConfig{
		ID:                  "config-123",
		EmailDomain:         "test.com",
		ConfigType:          client.AuthConfigTypeClient,
		PrimaryClientID:     "client-456",
		AdditionalClientIDs: []string{"client-789"},
		AuthProvider:        client.AuthProviderOIDC,
		IdpType:             "ENTRA",
		OIDCIssuerURL:       "https://login.microsoftonline.com/tenant/v2.0",
		OIDCClientID:        "app-id",
		OIDCMultiTenant:     false,
		EntraTenantID:       "tenant-id",
		GroupsClaim:         "groups",
		RolesClaim:          "roles",
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	dto := toAuthConfigDTO(config)

	if dto.ID != config.ID {
		t.Errorf("Expected ID %s, got %s", config.ID, dto.ID)
	}
	if dto.EmailDomain != config.EmailDomain {
		t.Errorf("Expected EmailDomain %s, got %s", config.EmailDomain, dto.EmailDomain)
	}
	if dto.ConfigType != string(config.ConfigType) {
		t.Errorf("Expected ConfigType %s, got %s", config.ConfigType, dto.ConfigType)
	}
	if dto.AuthProvider != string(config.AuthProvider) {
		t.Errorf("Expected AuthProvider %s, got %s", config.AuthProvider, dto.AuthProvider)
	}
	if len(dto.AdditionalClientIDs) != 1 {
		t.Errorf("Expected 1 additional client ID, got %d", len(dto.AdditionalClientIDs))
	}
}

// Tests for AnchorDomainDTO

func TestAnchorDomainDTO_JSON(t *testing.T) {
	dto := AnchorDomainDTO{
		ID:        "anchor-123",
		Domain:    "flowcatalyst.tech",
		CreatedAt: "2024-01-01T00:00:00Z",
	}

	data, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("Failed to marshal AnchorDomainDTO: %v", err)
	}

	jsonStr := string(data)

	// Verify camelCase field names
	expectedFields := []string{`"id"`, `"domain"`, `"createdAt"`}
	for _, field := range expectedFields {
		if !contains(jsonStr, field) {
			t.Errorf("Expected %s in JSON, got %s", field, jsonStr)
		}
	}

	// Verify domain value
	if !contains(jsonStr, `"domain":"flowcatalyst.tech"`) {
		t.Errorf("Expected domain 'flowcatalyst.tech' in JSON, got %s", jsonStr)
	}
}

func TestCreateAnchorDomainRequest_JSON(t *testing.T) {
	jsonData := `{"domain": "example.com"}`

	var req CreateAnchorDomainRequest
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if req.Domain != "example.com" {
		t.Errorf("Expected Domain 'example.com', got '%s'", req.Domain)
	}
}

// Tests for ClientAuthConfig entity

func TestClientAuthConfig_IsOIDC(t *testing.T) {
	tests := []struct {
		provider client.AuthProvider
		expected bool
	}{
		{client.AuthProviderOIDC, true},
		{client.AuthProviderInternal, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.provider), func(t *testing.T) {
			config := &client.ClientAuthConfig{AuthProvider: tt.provider}
			if config.IsOIDC() != tt.expected {
				t.Errorf("Expected IsOIDC()=%v for provider %s", tt.expected, tt.provider)
			}
		})
	}
}

func TestClientAuthConfig_GetAllClientIDs(t *testing.T) {
	tests := []struct {
		name       string
		config     client.ClientAuthConfig
		expected   []string
		expectNil  bool
	}{
		{
			name: "anchor type returns nil",
			config: client.ClientAuthConfig{
				ConfigType: client.AuthConfigTypeAnchor,
			},
			expectNil: true,
		},
		{
			name: "partner type returns granted IDs",
			config: client.ClientAuthConfig{
				ConfigType:       client.AuthConfigTypePartner,
				GrantedClientIDs: []string{"client-1", "client-2"},
			},
			expected: []string{"client-1", "client-2"},
		},
		{
			name: "client type returns primary and additional",
			config: client.ClientAuthConfig{
				ConfigType:          client.AuthConfigTypeClient,
				PrimaryClientID:     "primary",
				AdditionalClientIDs: []string{"additional-1", "additional-2"},
			},
			expected: []string{"primary", "additional-1", "additional-2"},
		},
		{
			name: "client type with only primary",
			config: client.ClientAuthConfig{
				ConfigType:      client.AuthConfigTypeClient,
				PrimaryClientID: "only-primary",
			},
			expected: []string{"only-primary"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetAllClientIDs()
			if tt.expectNil {
				if result != nil {
					t.Errorf("Expected nil, got %v", result)
				}
				return
			}
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d IDs, got %d", len(tt.expected), len(result))
				return
			}
			for i, id := range tt.expected {
				if result[i] != id {
					t.Errorf("Expected ID[%d]=%s, got %s", i, id, result[i])
				}
			}
		})
	}
}

// Tests for AuthConfigType constants

func TestAuthConfigType_Constants(t *testing.T) {
	if client.AuthConfigTypeAnchor != "ANCHOR" {
		t.Errorf("Expected ANCHOR, got %s", client.AuthConfigTypeAnchor)
	}
	if client.AuthConfigTypePartner != "PARTNER" {
		t.Errorf("Expected PARTNER, got %s", client.AuthConfigTypePartner)
	}
	if client.AuthConfigTypeClient != "CLIENT" {
		t.Errorf("Expected CLIENT, got %s", client.AuthConfigTypeClient)
	}
}

// Tests for AuthProvider constants

func TestAuthProvider_Constants(t *testing.T) {
	if client.AuthProviderInternal != "INTERNAL" {
		t.Errorf("Expected INTERNAL, got %s", client.AuthProviderInternal)
	}
	if client.AuthProviderOIDC != "OIDC" {
		t.Errorf("Expected OIDC, got %s", client.AuthProviderOIDC)
	}
}

// Tests for AnchorDomain entity

func TestAnchorDomain_Structure(t *testing.T) {
	now := time.Now()
	domain := client.AnchorDomain{
		ID:        "anchor-1",
		Domain:    "admin.flowcatalyst.tech",
		CreatedAt: now,
	}

	if domain.ID != "anchor-1" {
		t.Errorf("Expected ID 'anchor-1', got '%s'", domain.ID)
	}
	if domain.Domain != "admin.flowcatalyst.tech" {
		t.Errorf("Expected Domain 'admin.flowcatalyst.tech', got '%s'", domain.Domain)
	}
}

// Tests for IdpRoleMapping entity

func TestIdpRoleMapping_Structure(t *testing.T) {
	now := time.Now()
	mapping := client.IdpRoleMapping{
		ID:           "mapping-1",
		EmailDomain:  "acme.com",
		IdpGroupName: "admins",
		RoleID:       "role-123",
		RoleName:     "Administrator",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if mapping.IdpGroupName != "admins" {
		t.Errorf("Expected IdpGroupName 'admins', got '%s'", mapping.IdpGroupName)
	}
	if mapping.RoleID != "role-123" {
		t.Errorf("Expected RoleID 'role-123', got '%s'", mapping.RoleID)
	}
}

// Test AuthConfigDTO with multi-tenant OIDC

func TestAuthConfigDTO_MultiTenant(t *testing.T) {
	dto := AuthConfigDTO{
		ID:                "config-multi",
		EmailDomain:       "acme.com",
		ConfigType:        "PARTNER",
		AuthProvider:      "OIDC",
		OIDCMultiTenant:   true,
		OIDCIssuerPattern: "https://login.microsoftonline.com/{tenantId}/v2.0",
		GrantedClientIDs:  []string{"client-1", "client-2"},
	}

	if !dto.OIDCMultiTenant {
		t.Error("Expected OIDCMultiTenant to be true")
	}

	if dto.OIDCIssuerPattern == "" {
		t.Error("Expected OIDCIssuerPattern to be set")
	}

	if len(dto.GrantedClientIDs) != 2 {
		t.Errorf("Expected 2 granted client IDs, got %d", len(dto.GrantedClientIDs))
	}
}

// Test CreateAuthConfigRequest validation patterns

func TestCreateAuthConfigRequest_OIDCValidation(t *testing.T) {
	tests := []struct {
		name       string
		request    CreateAuthConfigRequest
		hasIssuer  bool
		hasClient  bool
	}{
		{
			name: "complete OIDC config",
			request: CreateAuthConfigRequest{
				EmailDomain:   "test.com",
				AuthProvider:  "OIDC",
				OIDCIssuerURL: "https://issuer.example.com",
				OIDCClientID:  "client-id",
			},
			hasIssuer: true,
			hasClient: true,
		},
		{
			name: "missing issuer URL",
			request: CreateAuthConfigRequest{
				EmailDomain:  "test.com",
				AuthProvider: "OIDC",
				OIDCClientID: "client-id",
			},
			hasIssuer: false,
			hasClient: true,
		},
		{
			name: "internal auth no OIDC fields",
			request: CreateAuthConfigRequest{
				EmailDomain:  "test.com",
				AuthProvider: "INTERNAL",
			},
			hasIssuer: false,
			hasClient: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if (tt.request.OIDCIssuerURL != "") != tt.hasIssuer {
				t.Errorf("hasIssuer mismatch")
			}
			if (tt.request.OIDCClientID != "") != tt.hasClient {
				t.Errorf("hasClient mismatch")
			}
		})
	}
}

// Helper for tests
func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
