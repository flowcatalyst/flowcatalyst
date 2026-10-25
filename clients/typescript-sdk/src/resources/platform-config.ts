/**
 * Platform Config Resource
 *
 * Read and manage per-application platform configuration properties, and
 * reveal SECRET properties in plaintext.
 *
 * SECRET values: `list()` and `get()` ALWAYS return the literal `"***"` as the
 * `value` of a property whose `valueType` is `SECRET`, for every caller. The
 * only way to read a secret's plaintext is {@link PlatformConfigResource.revealSecret}.
 */

import type { ResultAsync } from "neverthrow";
import type { SdkError } from "../errors.js";
import type { FlowCatalystClient } from "../client.js";
import type {
	ConfigListResponse,
	ConfigResponse,
	SetPropertyRequest,
} from "../generated/types.gen.js";

export type { ConfigListResponse, ConfigResponse, SetPropertyRequest };

/** Options common to the per-property operations. */
export interface PlatformConfigScope {
	/** Target a client-scoped override instead of the GLOBAL value. */
	clientId?: string;
}

/**
 * A revealed secret. SENSITIVE: `value` is the plaintext.
 *
 * To keep it out of logs by accident, `value` is a NON-enumerable property:
 * it is omitted from `Object.keys`, object spread and `console.log` /
 * `util.inspect` output, and `JSON.stringify` / `String()` render it as
 * `"[REDACTED]"`. Read it explicitly with `secret.value`.
 */
export interface RevealedSecret {
	readonly applicationCode: string;
	readonly section: string;
	readonly property: string;
	readonly clientId?: string;
	/** The secret plaintext. Never log it. */
	readonly value: string;
	/** The value's version; changes whenever the secret is set (detects rotation). */
	readonly updatedAt: string;
}

const REDACTED = "[REDACTED]";
const INSPECT = Symbol.for("nodejs.util.inspect.custom");

/** Wrap the wire response so the plaintext does not leak via logging/serialisation. */
function toRevealedSecret(raw: {
	applicationCode: string;
	section: string;
	property: string;
	clientId?: string;
	value: string;
	updatedAt: string;
}): RevealedSecret {
	const meta = {
		applicationCode: raw.applicationCode,
		section: raw.section,
		property: raw.property,
		...(raw.clientId !== undefined ? { clientId: raw.clientId } : {}),
		updatedAt: raw.updatedAt,
	};
	const out: Record<string | symbol, unknown> = { ...meta };
	Object.defineProperty(out, "value", {
		value: raw.value,
		enumerable: false,
		writable: false,
		configurable: false,
	});
	const redacted = () => ({ ...meta, value: REDACTED });
	const hidden = (fn: unknown) => ({
		value: fn,
		enumerable: false,
		writable: false,
		configurable: false,
	});
	Object.defineProperty(out, "toJSON", hidden(redacted));
	Object.defineProperty(out, "toString", hidden(() => JSON.stringify(redacted())));
	Object.defineProperty(out, INSPECT, hidden(() => `RevealedSecret ${JSON.stringify(redacted())}`));
	return out as unknown as RevealedSecret;
}

function query(scope?: PlatformConfigScope): { clientId: string } | undefined {
	return scope?.clientId ? { clientId: scope.clientId } : undefined;
}

export class PlatformConfigResource {
	private readonly client: FlowCatalystClient;

	constructor(client: FlowCatalystClient) {
		this.client = client;
	}

	/**
	 * List the properties of an application.
	 *
	 * SECRET properties have `value === "***"`; use {@link revealSecret} for the plaintext.
	 */
	list(app: string): ResultAsync<ConfigListResponse, SdkError> {
		return this.client.request<ConfigListResponse>((httpClient, headers) =>
			httpClient.get({
				url: "/api/platform-config/{app}",
				headers,
				path: { app },
			}),
		);
	}

	/**
	 * Get a property.
	 *
	 * For a SECRET property `value` is always `"***"`; use {@link revealSecret} for the plaintext.
	 */
	get(
		app: string,
		section: string,
		property: string,
		scope?: PlatformConfigScope,
	): ResultAsync<ConfigResponse, SdkError> {
		return this.client.request<ConfigResponse>((httpClient, headers) =>
			httpClient.get({
				url: "/api/config/{app}/{section}/{property}",
				headers,
				path: { app, section, property },
				query: query(scope),
			}),
		);
	}

	/** Create or replace a property (`valueType: "SECRET"` stores it encrypted). */
	set(
		app: string,
		section: string,
		property: string,
		data: SetPropertyRequest,
	): ResultAsync<ConfigResponse, SdkError> {
		const { clientId, ...body } = data;
		return this.client.request<ConfigResponse>((httpClient, headers) =>
			httpClient.put({
				url: "/api/config/{app}/{section}/{property}",
				headers: { ...headers, "Content-Type": "application/json" },
				path: { app, section, property },
				query: query({ clientId }),
				body: { ...body, ...(clientId ? { clientId } : {}) },
			}),
		);
	}

	/** Delete a property. */
	delete(
		app: string,
		section: string,
		property: string,
		scope?: PlatformConfigScope,
	): ResultAsync<unknown, SdkError> {
		return this.client.request<unknown>((httpClient, headers) =>
			httpClient.delete({
				url: "/api/config/{app}/{section}/{property}",
				headers,
				path: { app, section, property },
				query: query(scope),
			}),
		);
	}

	/**
	 * Reveal a SECRET property's plaintext. This is the ONLY way to read one.
	 *
	 * `GET /api/config/{app}/{section}/{property}/secret[?clientId=]`
	 *
	 * Always a permission: `platform:admin:config:reveal` (a platform
	 * administrator), or `platform:application-service:config:reveal` (an
	 * application's service account; the `platform:application-service` role
	 * carries it). The platform confines a service account to its own
	 * applications and to the clients it is linked to (its explicit clients,
	 * else the clients its application is enabled for), plus the
	 * application-wide value. Every call is audited server-side.
	 *
	 * Errors: `forbidden` (403), `not_found` (404), and `http_error` with
	 * `status: 400` and `body.error === "NOT_A_SECRET"` when the property is
	 * PLAIN (use {@link get}).
	 *
	 * The call is NOT cached by the SDK and sends no caching headers; the
	 * response carries `Cache-Control: no-store`. Add your own TTL cache if you
	 * need one. Never log the returned `value`.
	 */
	revealSecret(
		app: string,
		section: string,
		property: string,
		clientId?: string,
	): ResultAsync<RevealedSecret, SdkError> {
		return this.client
			.request<Parameters<typeof toRevealedSecret>[0]>((httpClient, headers) =>
				httpClient.get({
					url: "/api/config/{app}/{section}/{property}/secret",
					headers,
					path: { app, section, property },
					query: query({ clientId }),
				}),
			)
			.map(toRevealedSecret);
	}
}
