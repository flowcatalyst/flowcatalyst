/**
 * Platform AI Agent Read-Only Role
 *
 * Read-only access to event types and subscriptions for AI agent integrations.
 */

import { makeRole, type RoleDefinition } from "../role-definition.js";
import {
	EVENT_TYPE_PERMISSIONS,
	SUBSCRIPTION_PERMISSIONS,
} from "../permissions/platform-admin.js";

/**
 * AI Agent Read-Only role.
 * Provides read-only access to event types and subscriptions for AI agents.
 */
export const PLATFORM_AI_AGENT_READONLY: RoleDefinition = makeRole(
	"PLATFORM_AI_AGENT_READONLY",
	"AI Agent Read-Only",
	"Read-only access to event types and subscriptions for AI agent integrations",
	[EVENT_TYPE_PERMISSIONS.READ, SUBSCRIPTION_PERMISSIONS.READ],
);
