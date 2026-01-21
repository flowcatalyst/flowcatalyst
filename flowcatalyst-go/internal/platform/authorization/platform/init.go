package platform

import (
	"log/slog"

	"go.flowcatalyst.tech/internal/platform/authorization"
)

// RegisterAll registers all platform permissions and roles with the given registry.
// Permissions must be registered before roles that depend on them.
func RegisterAll(registry *authorization.PermissionRegistry, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info("Registering platform permissions and roles...")

	// Register all permissions first
	for _, perm := range AllIamPermissions() {
		registry.RegisterPermission(perm)
	}
	for _, perm := range AllAdminPermissions() {
		registry.RegisterPermission(perm)
	}
	for _, perm := range AllMessagingPermissions() {
		registry.RegisterPermission(perm)
	}
	for _, perm := range AllApplicationServicePermissions() {
		registry.RegisterPermission(perm)
	}

	// Register all roles (after permissions)
	for _, role := range AllRoles() {
		if err := registry.RegisterRole(role); err != nil {
			return err
		}
	}

	logger.Info("Platform registration complete",
		"permissions", registry.PermissionCount(),
		"roles", registry.RoleCount(),
	)
	return nil
}

// RegisterAllToDefault registers all platform permissions and roles with the default registry.
func RegisterAllToDefault() error {
	return RegisterAll(authorization.DefaultRegistry, nil)
}
