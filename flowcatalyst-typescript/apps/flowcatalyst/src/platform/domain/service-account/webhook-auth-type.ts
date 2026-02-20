/**
 * Webhook Authentication Type
 *
 * Determines how the platform authenticates when dispatching webhooks
 * to a service account's target endpoint.
 */

export const WebhookAuthType = {
	NONE: "NONE",
	BEARER_TOKEN: "BEARER_TOKEN",
	BASIC_AUTH: "BASIC_AUTH",
	API_KEY: "API_KEY",
	HMAC_SIGNATURE: "HMAC_SIGNATURE",
} as const;

export type WebhookAuthType =
	(typeof WebhookAuthType)[keyof typeof WebhookAuthType];
