// Package authorization provides RBAC (Role-Based Access Control) functionality.
//
// Permission format: {application}:{context}:{aggregate}:{action}
// Role format: {application}:{role-name}
//
// This package provides code-first permission and role definitions that are
// registered in an in-memory registry at startup.
package authorization

import (
	"fmt"
	"regexp"
)

// validPartPattern matches lowercase alphanumeric with hyphens (not at start/end)
var validPartPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// PermissionDefinition defines a permission in the FlowCatalyst system.
//
// Permissions follow the structure: {application}:{context}:{aggregate}:{action}
//
// Where:
//   - application: The registered application (e.g., "platform", "tms", "operant")
//   - context: Bounded context within the app (e.g., "iam", "admin", "messaging", "dispatch")
//   - aggregate: The entity/resource being accessed (e.g., "user", "role", "order")
//   - action: The operation (e.g., "view", "create", "update", "delete")
//
// Examples:
//   - platform:iam:user:create
//   - platform:admin:client:view
//   - platform:messaging:event-type:create
//   - tms:dispatch:order:update
type PermissionDefinition interface {
	// Application returns the application code (e.g., "platform", "tms")
	Application() string

	// Context returns the bounded context within app (e.g., "iam", "admin", "messaging")
	Context() string

	// Aggregate returns the resource/entity (e.g., "user", "role", "order")
	Aggregate() string

	// Action returns the operation (e.g., "view", "create", "update", "delete")
	Action() string

	// Description returns a human-readable description
	Description() string

	// ToPermissionString generates the string representation of this permission.
	// Format: {application}:{context}:{aggregate}:{action}
	ToPermissionString() string
}

// PermissionRecord is a concrete implementation of PermissionDefinition.
type PermissionRecord struct {
	application string
	context     string
	aggregate   string
	action      string
	description string
}

// NewPermission creates a new permission definition.
// All parts must be lowercase alphanumeric with hyphens allowed (not at start/end).
func NewPermission(application, context, aggregate, action, description string) (*PermissionRecord, error) {
	if err := validatePart(application, "application"); err != nil {
		return nil, err
	}
	if err := validatePart(context, "context"); err != nil {
		return nil, err
	}
	if err := validatePart(aggregate, "aggregate"); err != nil {
		return nil, err
	}
	if err := validatePart(action, "action"); err != nil {
		return nil, err
	}
	if description == "" {
		return nil, fmt.Errorf("description cannot be empty")
	}

	return &PermissionRecord{
		application: application,
		context:     context,
		aggregate:   aggregate,
		action:      action,
		description: description,
	}, nil
}

// MustPermission creates a new permission definition, panicking on error.
// Use this for compile-time defined permissions where validation errors indicate a bug.
func MustPermission(application, context, aggregate, action, description string) *PermissionRecord {
	p, err := NewPermission(application, context, aggregate, action, description)
	if err != nil {
		panic(fmt.Sprintf("invalid permission definition: %v", err))
	}
	return p
}

// Application returns the application code
func (p *PermissionRecord) Application() string {
	return p.application
}

// Context returns the bounded context
func (p *PermissionRecord) Context() string {
	return p.context
}

// Aggregate returns the resource/entity
func (p *PermissionRecord) Aggregate() string {
	return p.aggregate
}

// Action returns the operation
func (p *PermissionRecord) Action() string {
	return p.action
}

// Description returns the human-readable description
func (p *PermissionRecord) Description() string {
	return p.description
}

// ToPermissionString generates the string representation.
// Format: {application}:{context}:{aggregate}:{action}
func (p *PermissionRecord) ToPermissionString() string {
	return fmt.Sprintf("%s:%s:%s:%s", p.application, p.context, p.aggregate, p.action)
}

// String returns a human-readable representation
func (p *PermissionRecord) String() string {
	return fmt.Sprintf("%s (%s)", p.ToPermissionString(), p.description)
}

// validatePart validates that a part follows the naming rules:
//   - Lowercase letters, numbers, and hyphens only
//   - Cannot start or end with a hyphen
//   - At least 1 character
func validatePart(part, partName string) error {
	if part == "" {
		return fmt.Errorf("%s cannot be empty", partName)
	}
	if !validPartPattern.MatchString(part) {
		return fmt.Errorf("%s must be lowercase alphanumeric with hyphens (cannot start/end with hyphen): %s", partName, part)
	}
	return nil
}

// ParsePermissionString parses a permission string into its components.
// Format: application:context:aggregate:action
func ParsePermissionString(permissionString string) (*PermissionRecord, error) {
	parts := splitPermissionString(permissionString)
	if len(parts) != 4 {
		return nil, fmt.Errorf("invalid permission string format (expected 4 parts): %s", permissionString)
	}
	return NewPermission(parts[0], parts[1], parts[2], parts[3], "Parsed from string")
}

// splitPermissionString splits a permission string by colon
func splitPermissionString(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}
