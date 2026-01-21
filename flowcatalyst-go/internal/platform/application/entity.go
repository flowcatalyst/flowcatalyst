package application

import (
	"time"
)

// ApplicationType defines the type of application
type ApplicationType string

const (
	ApplicationTypeApplication ApplicationType = "APPLICATION" // Regular application
	ApplicationTypeIntegration ApplicationType = "INTEGRATION" // Integration/connector
)

// Application represents a registered application in the platform
// Collection: auth_applications
type Application struct {
	ID               string          `bson:"_id" json:"id"`
	Type             ApplicationType `bson:"type" json:"type"`
	Code             string          `bson:"code" json:"code"` // Unique application code
	Name             string          `bson:"name" json:"name"`
	Description      string          `bson:"description,omitempty" json:"description,omitempty"`
	IconURL          string          `bson:"iconUrl,omitempty" json:"iconUrl,omitempty"`
	DefaultBaseURL   string          `bson:"defaultBaseUrl,omitempty" json:"defaultBaseUrl,omitempty"`
	ServiceAccountID string          `bson:"serviceAccountId,omitempty" json:"serviceAccountId,omitempty"`
	Active           bool            `bson:"active" json:"active"`
	CreatedAt        time.Time       `bson:"createdAt" json:"createdAt"`
	UpdatedAt        time.Time       `bson:"updatedAt" json:"updatedAt"`
}

// ApplicationClientConfig represents per-client configuration for an application
// Collection: application_client_config
type ApplicationClientConfig struct {
	ID              string    `bson:"_id" json:"id"`
	ApplicationID   string    `bson:"applicationId" json:"applicationId"`
	ClientID        string    `bson:"clientId" json:"clientId"`
	Enabled         bool      `bson:"enabled" json:"enabled"`
	BaseURLOverride string    `bson:"baseUrlOverride,omitempty" json:"baseUrlOverride,omitempty"`
	ConfigJSON      string    `bson:"configJson,omitempty" json:"configJson,omitempty"`
	CreatedAt       time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt       time.Time `bson:"updatedAt" json:"updatedAt"`
}

// GetBaseURL returns the effective base URL for this client config
func (c *ApplicationClientConfig) GetBaseURL(app *Application) string {
	if c.BaseURLOverride != "" {
		return c.BaseURLOverride
	}
	return app.DefaultBaseURL
}

// RoleSource defines where a role was defined
type RoleSource string

const (
	RoleSourceCode     RoleSource = "CODE"     // Defined in code
	RoleSourceDatabase RoleSource = "DATABASE" // Defined in database
	RoleSourceSDK      RoleSource = "SDK"      // Registered via SDK
)

// AuthRole represents a role in the RBAC system
// Collection: auth_roles
type AuthRole struct {
	ID              string     `bson:"_id" json:"id"`
	ApplicationID   string     `bson:"applicationId,omitempty" json:"applicationId,omitempty"`
	ApplicationCode string     `bson:"applicationCode,omitempty" json:"applicationCode,omitempty"`
	Name            string     `bson:"name" json:"name"` // Unique role name
	DisplayName     string     `bson:"displayName" json:"displayName"`
	Description     string     `bson:"description,omitempty" json:"description,omitempty"`
	Permissions     []string   `bson:"permissions,omitempty" json:"permissions,omitempty"`
	Source          RoleSource `bson:"source" json:"source"`
	ClientManaged   bool       `bson:"clientManaged" json:"clientManaged"`
	CreatedAt       time.Time  `bson:"createdAt" json:"createdAt"`
	UpdatedAt       time.Time  `bson:"updatedAt" json:"updatedAt"`
}

// HasPermission checks if the role has a specific permission
func (r *AuthRole) HasPermission(permission string) bool {
	for _, p := range r.Permissions {
		if p == permission {
			return true
		}
	}
	return false
}

// PermissionSource defines where a permission was defined
type PermissionSource string

const (
	PermissionSourceSDK      PermissionSource = "SDK"
	PermissionSourceDatabase PermissionSource = "DATABASE"
)

// AuthPermission represents a permission in the RBAC system
// Collection: auth_permissions
type AuthPermission struct {
	ID            string           `bson:"_id" json:"id"`
	ApplicationID string           `bson:"applicationId,omitempty" json:"applicationId,omitempty"`
	Name          string           `bson:"name" json:"name"` // Unique permission name
	DisplayName   string           `bson:"displayName" json:"displayName"`
	Description   string           `bson:"description,omitempty" json:"description,omitempty"`
	Source        PermissionSource `bson:"source" json:"source"`
	CreatedAt     time.Time        `bson:"createdAt" json:"createdAt"`
}
