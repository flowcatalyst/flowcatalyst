/**
 * Identity Provider Domain Exports
 */

export type { IdentityProviderType } from "./identity-provider-type.js";
export {
	type IdentityProvider,
	type NewIdentityProvider,
	createIdentityProvider,
	updateIdentityProvider,
	getEffectiveIssuerPattern,
	isValidIssuer as isIdpValidIssuer,
	isEmailDomainAllowed,
	validateOidcConfig as validateIdpOidcConfig,
	hasClientSecret,
} from "./identity-provider.js";
export {
	IdentityProviderCreated,
	IdentityProviderUpdated,
	IdentityProviderDeleted,
} from "./events.js";
