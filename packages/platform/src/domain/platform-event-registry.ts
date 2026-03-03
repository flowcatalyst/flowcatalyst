/**
 * Platform Event Type Registry
 *
 * Static registry of all domain events emitted by the platform.
 * Used by the "Sync Platform Events" action to register event types
 * with the event-type catalogue so downstream consumers can discover them.
 *
 * Format: { subdomain, aggregate, event, name }
 * Code is derived as: platform:{subdomain}:{aggregate}:{event}
 */

export interface PlatformEventDefinition {
	readonly subdomain: string;
	readonly aggregate: string;
	readonly event: string;
	readonly name: string;
}

function titleCase(s: string): string {
	return s
		.split("-")
		.map((w) => w.charAt(0).toUpperCase() + w.slice(1))
		.join(" ");
}

function eventName(aggregate: string, event: string): string {
	return `${titleCase(aggregate)} ${titleCase(event)}`;
}

/**
 * All platform domain events, grouped by subdomain.
 */
export const PLATFORM_EVENT_DEFINITIONS: readonly PlatformEventDefinition[] = [
	// ─── IAM ──────────────────────────────────────────────────────────────
	// User
	{ subdomain: "iam", aggregate: "user", event: "created", name: eventName("user", "created") },
	{ subdomain: "iam", aggregate: "user", event: "updated", name: eventName("user", "updated") },
	{ subdomain: "iam", aggregate: "user", event: "activated", name: eventName("user", "activated") },
	{ subdomain: "iam", aggregate: "user", event: "deactivated", name: eventName("user", "deactivated") },
	{ subdomain: "iam", aggregate: "user", event: "deleted", name: eventName("user", "deleted") },
	{ subdomain: "iam", aggregate: "user", event: "roles-assigned", name: eventName("user", "roles-assigned") },
	{ subdomain: "iam", aggregate: "user", event: "application-access-assigned", name: eventName("user", "application-access-assigned") },
	{ subdomain: "iam", aggregate: "user", event: "client-access-granted", name: eventName("user", "client-access-granted") },
	{ subdomain: "iam", aggregate: "user", event: "client-access-revoked", name: eventName("user", "client-access-revoked") },
	{ subdomain: "iam", aggregate: "user", event: "logged-in", name: eventName("user", "logged-in") },
	{ subdomain: "iam", aggregate: "user", event: "password-reset-requested", name: eventName("user", "password-reset-requested") },
	{ subdomain: "iam", aggregate: "user", event: "password-reset", name: eventName("user", "password-reset") },
	{ subdomain: "iam", aggregate: "user", event: "principals-synced", name: eventName("user", "principals-synced") },
	// Service Account
	{ subdomain: "iam", aggregate: "service-account", event: "created", name: eventName("service-account", "created") },
	{ subdomain: "iam", aggregate: "service-account", event: "updated", name: eventName("service-account", "updated") },
	{ subdomain: "iam", aggregate: "service-account", event: "auth-token-regenerated", name: eventName("service-account", "auth-token-regenerated") },
	{ subdomain: "iam", aggregate: "service-account", event: "signing-secret-regenerated", name: eventName("service-account", "signing-secret-regenerated") },
	{ subdomain: "iam", aggregate: "service-account", event: "deleted", name: eventName("service-account", "deleted") },
	// OAuth Client
	{ subdomain: "iam", aggregate: "oauth-client", event: "created", name: eventName("oauth-client", "created") },
	{ subdomain: "iam", aggregate: "oauth-client", event: "updated", name: eventName("oauth-client", "updated") },
	{ subdomain: "iam", aggregate: "oauth-client", event: "secret-regenerated", name: eventName("oauth-client", "secret-regenerated") },
	{ subdomain: "iam", aggregate: "oauth-client", event: "deleted", name: eventName("oauth-client", "deleted") },
	// Identity Provider
	{ subdomain: "iam", aggregate: "identity-provider", event: "created", name: eventName("identity-provider", "created") },
	{ subdomain: "iam", aggregate: "identity-provider", event: "updated", name: eventName("identity-provider", "updated") },
	{ subdomain: "iam", aggregate: "identity-provider", event: "deleted", name: eventName("identity-provider", "deleted") },
	// Auth Config
	{ subdomain: "iam", aggregate: "auth-config", event: "created", name: eventName("auth-config", "created") },
	{ subdomain: "iam", aggregate: "auth-config", event: "updated", name: eventName("auth-config", "updated") },
	{ subdomain: "iam", aggregate: "auth-config", event: "deleted", name: eventName("auth-config", "deleted") },
	// Email Domain Mapping
	{ subdomain: "iam", aggregate: "email-domain-mapping", event: "created", name: eventName("email-domain-mapping", "created") },
	{ subdomain: "iam", aggregate: "email-domain-mapping", event: "updated", name: eventName("email-domain-mapping", "updated") },
	{ subdomain: "iam", aggregate: "email-domain-mapping", event: "deleted", name: eventName("email-domain-mapping", "deleted") },

	// ─── Admin ────────────────────────────────────────────────────────────
	// Client
	{ subdomain: "admin", aggregate: "client", event: "created", name: eventName("client", "created") },
	{ subdomain: "admin", aggregate: "client", event: "updated", name: eventName("client", "updated") },
	{ subdomain: "admin", aggregate: "client", event: "status-changed", name: eventName("client", "status-changed") },
	{ subdomain: "admin", aggregate: "client", event: "deleted", name: eventName("client", "deleted") },
	{ subdomain: "admin", aggregate: "client", event: "note-added", name: eventName("client", "note-added") },
	// Anchor Domain
	{ subdomain: "admin", aggregate: "anchor-domain", event: "created", name: eventName("anchor-domain", "created") },
	{ subdomain: "admin", aggregate: "anchor-domain", event: "updated", name: eventName("anchor-domain", "updated") },
	{ subdomain: "admin", aggregate: "anchor-domain", event: "deleted", name: eventName("anchor-domain", "deleted") },
	// Application
	{ subdomain: "admin", aggregate: "application", event: "created", name: eventName("application", "created") },
	{ subdomain: "admin", aggregate: "application", event: "updated", name: eventName("application", "updated") },
	{ subdomain: "admin", aggregate: "application", event: "activated", name: eventName("application", "activated") },
	{ subdomain: "admin", aggregate: "application", event: "deactivated", name: eventName("application", "deactivated") },
	{ subdomain: "admin", aggregate: "application", event: "deleted", name: eventName("application", "deleted") },
	{ subdomain: "admin", aggregate: "application", event: "enabled-for-client", name: eventName("application", "enabled-for-client") },
	{ subdomain: "admin", aggregate: "application", event: "disabled-for-client", name: eventName("application", "disabled-for-client") },
	// Role
	{ subdomain: "admin", aggregate: "role", event: "created", name: eventName("role", "created") },
	{ subdomain: "admin", aggregate: "role", event: "updated", name: eventName("role", "updated") },
	{ subdomain: "admin", aggregate: "role", event: "deleted", name: eventName("role", "deleted") },
	{ subdomain: "admin", aggregate: "role", event: "synced", name: eventName("role", "synced") },
	// CORS Origin
	{ subdomain: "admin", aggregate: "cors-origin", event: "added", name: eventName("cors-origin", "added") },
	{ subdomain: "admin", aggregate: "cors-origin", event: "deleted", name: eventName("cors-origin", "deleted") },
	// Event Type
	{ subdomain: "admin", aggregate: "event-type", event: "created", name: eventName("event-type", "created") },
	{ subdomain: "admin", aggregate: "event-type", event: "updated", name: eventName("event-type", "updated") },
	{ subdomain: "admin", aggregate: "event-type", event: "archived", name: eventName("event-type", "archived") },
	{ subdomain: "admin", aggregate: "event-type", event: "deleted", name: eventName("event-type", "deleted") },
	{ subdomain: "admin", aggregate: "event-type", event: "schema-added", name: eventName("event-type", "schema-added") },
	{ subdomain: "admin", aggregate: "event-type", event: "schema-finalised", name: eventName("event-type", "schema-finalised") },
	{ subdomain: "admin", aggregate: "event-type", event: "schema-deprecated", name: eventName("event-type", "schema-deprecated") },
	{ subdomain: "admin", aggregate: "event-type", event: "synced", name: eventName("event-type", "synced") },

	// ─── Control Plane ────────────────────────────────────────────────────
	// Dispatch Pool
	{ subdomain: "control-plane", aggregate: "dispatch-pool", event: "created", name: eventName("dispatch-pool", "created") },
	{ subdomain: "control-plane", aggregate: "dispatch-pool", event: "updated", name: eventName("dispatch-pool", "updated") },
	{ subdomain: "control-plane", aggregate: "dispatch-pool", event: "deleted", name: eventName("dispatch-pool", "deleted") },
	{ subdomain: "control-plane", aggregate: "dispatch-pool", event: "synced", name: eventName("dispatch-pool", "synced") },
	// Subscription
	{ subdomain: "control-plane", aggregate: "subscription", event: "created", name: eventName("subscription", "created") },
	{ subdomain: "control-plane", aggregate: "subscription", event: "updated", name: eventName("subscription", "updated") },
	{ subdomain: "control-plane", aggregate: "subscription", event: "deleted", name: eventName("subscription", "deleted") },
	// Subscription Sync (uses "messaging" subdomain override)
	{ subdomain: "messaging", aggregate: "subscription", event: "synced", name: "Subscriptions Synced" },
];
