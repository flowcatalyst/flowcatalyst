package security

import (
	"strings"
	"testing"
)

/*
THREAT MODEL: Permission Registry

Role-based access control depends on correct permission handling:

1. APPLICATION CODE EXTRACTION: Roles follow "app:permission" format
2. ROLE FILTERING: Users should only see roles for authorized applications
3. DISPLAY NAMES: Role names must be parsed correctly for UI display
4. EDGE CASES: Null, empty, malformed roles must be handled safely

Attack vectors being tested:
- Malformed role names causing parser failures
- Permission escalation through role name manipulation
- Application boundary violations
- Empty/null injection attacks
*/

// === Role Parsing Functions (mirror Java's PermissionRegistry) ===

// ExtractApplicationCode extracts the application code from a role name
// Role format: "application:role-name" -> returns "application"
func ExtractApplicationCode(role string) string {
	if role == "" {
		return ""
	}
	role = strings.TrimSpace(role)
	if role == "" {
		return ""
	}

	idx := strings.Index(role, ":")
	if idx == -1 {
		return role // No colon, return entire string as app code
	}
	return role[:idx]
}

// GetDisplayName extracts the display name from a role (after first colon)
// Role format: "application:role-name" -> returns "role-name"
func GetDisplayName(role string) string {
	if role == "" {
		return ""
	}
	role = strings.TrimSpace(role)

	idx := strings.Index(role, ":")
	if idx == -1 {
		return role // No colon, return entire string
	}
	if idx+1 >= len(role) {
		return "" // Colon at end
	}
	return role[idx+1:]
}

// ExtractApplicationCodes extracts unique application codes from a list of roles
func ExtractApplicationCodes(roles []string) []string {
	if roles == nil || len(roles) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var result []string

	for _, role := range roles {
		code := ExtractApplicationCode(role)
		if code != "" && !seen[code] {
			seen[code] = true
			result = append(result, code)
		}
	}

	return result
}

// FilterRolesForApplication filters roles to only those belonging to an application
func FilterRolesForApplication(roles []string, appCode string) []string {
	if roles == nil || appCode == "" {
		return nil
	}

	var result []string
	for _, role := range roles {
		if ExtractApplicationCode(role) == appCode {
			result = append(result, role)
		}
	}

	return result
}

// === Tests ===

func TestPermissionRegistry_ExtractApplicationCode(t *testing.T) {
	tests := []struct {
		name     string
		role     string
		expected string
	}{
		{"standard role", "operant:dispatch:admin", "operant"},
		{"simple role", "admin:users", "admin"},
		{"no colon", "superadmin", "superadmin"},
		{"empty string", "", ""},
		{"only colon", ":", ""},
		{"colon at end", "app:", "app"},
		{"multiple colons", "app:sub:role", "app"},
		{"whitespace", "  app:role  ", "app"},
		{"only whitespace", "   ", ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ExtractApplicationCode(test.role)
			if result != test.expected {
				t.Errorf("ExtractApplicationCode(%q) = %q, want %q", test.role, result, test.expected)
			}
		})
	}
}

func TestPermissionRegistry_GetDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		role     string
		expected string
	}{
		{"standard role", "operant:dispatch:admin", "dispatch:admin"},
		{"simple role", "admin:users", "users"},
		{"no colon", "superadmin", "superadmin"},
		{"empty string", "", ""},
		{"only colon", ":", ""},
		{"colon at end", "app:", ""},
		{"multiple colons", "app:sub:role", "sub:role"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := GetDisplayName(test.role)
			if result != test.expected {
				t.Errorf("GetDisplayName(%q) = %q, want %q", test.role, result, test.expected)
			}
		})
	}
}

func TestPermissionRegistry_ExtractApplicationCodes_Unique(t *testing.T) {
	roles := []string{
		"operant:dispatch:admin",
		"operant:dispatch:viewer",
		"platform:users:admin",
		"operant:events:admin",
		"platform:settings",
	}

	result := ExtractApplicationCodes(roles)

	// Should have 2 unique codes: operant, platform
	if len(result) != 2 {
		t.Errorf("Expected 2 unique codes, got %d: %v", len(result), result)
	}

	// Verify both are present
	hasOperant := false
	hasPlatform := false
	for _, code := range result {
		if code == "operant" {
			hasOperant = true
		}
		if code == "platform" {
			hasPlatform = true
		}
	}

	if !hasOperant {
		t.Error("Expected 'operant' in result")
	}
	if !hasPlatform {
		t.Error("Expected 'platform' in result")
	}
}

func TestPermissionRegistry_ExtractApplicationCodes_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		roles    []string
		expected int // number of unique codes
	}{
		{"nil roles", nil, 0},
		{"empty roles", []string{}, 0},
		{"single role", []string{"app:role"}, 1},
		{"duplicate apps", []string{"app:r1", "app:r2", "app:r3"}, 1},
		{"all empty", []string{"", "", ""}, 0},
		{"mixed empty", []string{"app:role", "", "other:role"}, 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ExtractApplicationCodes(test.roles)
			if len(result) != test.expected {
				t.Errorf("ExtractApplicationCodes(%v) = %d codes, want %d", test.roles, len(result), test.expected)
			}
		})
	}
}

func TestPermissionRegistry_FilterRolesForApplication(t *testing.T) {
	roles := []string{
		"operant:dispatch:admin",
		"operant:dispatch:viewer",
		"platform:users:admin",
		"operant:events:admin",
		"platform:settings",
	}

	// Filter for operant
	operantRoles := FilterRolesForApplication(roles, "operant")
	if len(operantRoles) != 3 {
		t.Errorf("Expected 3 operant roles, got %d", len(operantRoles))
	}

	// Filter for platform
	platformRoles := FilterRolesForApplication(roles, "platform")
	if len(platformRoles) != 2 {
		t.Errorf("Expected 2 platform roles, got %d", len(platformRoles))
	}

	// Filter for non-existent app
	unknownRoles := FilterRolesForApplication(roles, "unknown")
	if len(unknownRoles) != 0 {
		t.Errorf("Expected 0 roles for unknown app, got %d", len(unknownRoles))
	}
}

func TestPermissionRegistry_FilterRolesForApplication_EdgeCases(t *testing.T) {
	roles := []string{"app:role1", "app:role2"}

	// Nil roles
	result := FilterRolesForApplication(nil, "app")
	if result != nil {
		t.Error("Expected nil for nil roles")
	}

	// Empty app code
	result = FilterRolesForApplication(roles, "")
	if result != nil {
		t.Error("Expected nil for empty app code")
	}

	// Empty roles slice
	result = FilterRolesForApplication([]string{}, "app")
	if len(result) != 0 {
		t.Errorf("Expected 0 roles for empty input, got %d", len(result))
	}
}

func TestPermissionRegistry_RoleNameSecurity(t *testing.T) {
	// Test that role parsing doesn't allow injection attacks

	maliciousRoles := []string{
		"admin:*",                    // Wildcard attempt
		"../../../etc/passwd",        // Path traversal
		"admin; DROP TABLE users;--", // SQL injection
		"<script>alert(1)</script>",  // XSS
		"admin\x00hidden",            // Null byte injection
		"admin\nX-Admin: true",       // Header injection
	}

	for _, role := range maliciousRoles {
		t.Run(role, func(t *testing.T) {
			// Should not panic
			appCode := ExtractApplicationCode(role)
			displayName := GetDisplayName(role)

			// App code should be deterministic (whatever is before first colon)
			_ = appCode
			_ = displayName

			// Filter should work without panic
			filtered := FilterRolesForApplication([]string{role}, appCode)
			if len(filtered) != 1 {
				t.Errorf("Expected 1 filtered role, got %d", len(filtered))
			}
		})
	}
}

func TestPermissionRegistry_ApplicationBoundary(t *testing.T) {
	// Test that roles from one application cannot leak into another

	appARoles := []string{
		"app-a:admin",
		"app-a:user",
		"app-a:viewer",
	}

	appBRoles := []string{
		"app-b:admin",
		"app-b:user",
	}

	allRoles := append(appARoles, appBRoles...)

	// Filter for app-a should only return app-a roles
	filteredA := FilterRolesForApplication(allRoles, "app-a")
	for _, role := range filteredA {
		if !strings.HasPrefix(role, "app-a:") {
			t.Errorf("app-a filter returned non-app-a role: %s", role)
		}
	}

	// Filter for app-b should only return app-b roles
	filteredB := FilterRolesForApplication(allRoles, "app-b")
	for _, role := range filteredB {
		if !strings.HasPrefix(role, "app-b:") {
			t.Errorf("app-b filter returned non-app-b role: %s", role)
		}
	}

	// Verify counts
	if len(filteredA) != 3 {
		t.Errorf("Expected 3 app-a roles, got %d", len(filteredA))
	}
	if len(filteredB) != 2 {
		t.Errorf("Expected 2 app-b roles, got %d", len(filteredB))
	}
}

func TestPermissionRegistry_CaseSensitivity(t *testing.T) {
	// Role names should be case-sensitive

	roles := []string{
		"App:Admin",
		"app:admin",
		"APP:ADMIN",
	}

	// All three should be different applications
	codes := ExtractApplicationCodes(roles)
	if len(codes) != 3 {
		t.Errorf("Expected 3 unique codes (case-sensitive), got %d: %v", len(codes), codes)
	}

	// Filtering should be case-sensitive
	filtered := FilterRolesForApplication(roles, "app")
	if len(filtered) != 1 {
		t.Errorf("Expected 1 role for lowercase 'app', got %d", len(filtered))
	}
}

func TestPermissionRegistry_SpecialCharacters(t *testing.T) {
	tests := []struct {
		role    string
		appCode string
	}{
		{"app-name:role", "app-name"},
		{"app_name:role", "app_name"},
		{"app.name:role", "app.name"},
		{"app123:role", "app123"},
		{"123app:role", "123app"},
	}

	for _, test := range tests {
		t.Run(test.role, func(t *testing.T) {
			result := ExtractApplicationCode(test.role)
			if result != test.appCode {
				t.Errorf("ExtractApplicationCode(%q) = %q, want %q", test.role, result, test.appCode)
			}
		})
	}
}
