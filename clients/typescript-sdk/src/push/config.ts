/**
 * What an application declares about its push topics: who may listen without
 * a ticket (rules) and the public keys its tickets are signed with. Synced to
 * the push gateway (docs/push-gateway-spec.md section 8) as a declarative
 * full replace: the payload IS the set, so send both halves every time.
 */

import { TicketMinter, type TicketJwks } from "./tickets.js";

/**
 * A topic rule (push contract section 6.3). A pattern's segments are literals
 * or placeholders: `{principal}` must equal the subscriber's principal id,
 * `{client}` the stream's client, any other `{name}` matches one segment.
 * The first segment must be the application's code.
 */
export interface PushRuleDefinition {
	/** `{application}:{segment}(:{segment})*` */
	pattern: string;
	/** Permissions the subscriber must all hold. */
	requires?: string[];
	/**
	 * A deliberately open topic: anyone may listen, with or without
	 * credentials. Required for a rule with neither `{principal}` nor
	 * `requires`; a public rule cannot also require permissions.
	 */
	public?: boolean;
}

/** The application's push declaration: rules and ticket keys. */
export interface PushDefinition {
	rules: PushRuleDefinition[];
	/** The ticket verification keys: a minter's JWKS, or a JWKS of Ed25519 public keys (at most 4). */
	keys: TicketJwks | TicketMinter;
}

/** The JWKS of a {@link PushDefinition}'s keys. */
export function pushKeysJwks(def: PushDefinition): TicketJwks {
	return def.keys instanceof TicketMinter ? def.keys.jwks() : def.keys;
}
