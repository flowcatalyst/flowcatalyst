/**
 * BFF Contract Types
 *
 * Shared types for the Backend-for-Frontend API surface.
 * These types are the single source of truth, derived from
 * the TypeBox schemas that also provide runtime validation.
 *
 * Consumers (e.g. the platform-frontend) import from
 * "@flowcatalyst/platform/bff" to get these types.
 */

// ─── Event Types ─────────────────────────────────────────────────────────────

export type {
	BffEventType as EventType,
	BffSpecVersion as SpecVersion,
	BffEventTypeListResponse as EventTypeListResponse,
	BffFilterOptionsResponse as FilterOptionsResponse,
	BffCreateEventTypeRequest as CreateEventTypeRequest,
	BffUpdateEventTypeRequest as UpdateEventTypeRequest,
	BffAddSchemaRequest as AddSchemaRequest,
} from "./event-types.js";

// ─── Roles & Permissions ─────────────────────────────────────────────────────

export type {
	BffRole as Role,
	BffPermission as Permission,
	BffRoleListResponse as RoleListResponse,
	BffApplicationOption as ApplicationOption,
	BffApplicationOptionsResponse as ApplicationOptionsResponse,
	BffCreateRoleRequest as CreateRoleRequest,
	BffUpdateRoleRequest as UpdateRoleRequest,
	BffPermissionListResponse as PermissionListResponse,
} from "./roles.js";

// ─── Derived Enum Types ──────────────────────────────────────────────────────
//
// Standalone union types extracted from response objects.
// These are derived from the TypeBox literal unions, so they
// stay in sync automatically.

import type { BffEventType, BffSpecVersion } from "./event-types.js";
import type { BffRole } from "./roles.js";

export type EventTypeStatus = BffEventType["status"];
export type SchemaType = BffSpecVersion["schemaType"];
export type SpecVersionStatus = BffSpecVersion["status"];
export type RoleSource = BffRole["source"];
