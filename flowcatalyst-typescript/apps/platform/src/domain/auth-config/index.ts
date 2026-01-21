/**
 * Auth Config Domain
 *
 * Domain models for client authentication configuration.
 */

export { type AuthConfigType, AuthConfigType as AuthConfigTypeValues } from './auth-config-type.js';
export { type AuthProvider, AuthProvider as AuthProviderValues } from './auth-provider.js';
export {
	type ClientAuthConfig,
	type NewClientAuthConfig,
	type CreateInternalAuthConfigInput,
	type CreateOidcAuthConfigInput,
	createInternalAuthConfig,
	createOidcAuthConfig,
	validateOidcConfig,
	validateConfigTypeConstraints,
	getAllAccessibleClientIds,
	isValidIssuer,
} from './client-auth-config.js';
export {
	type AuthConfigCreatedData,
	AuthConfigCreated,
	type AuthConfigUpdatedData,
	AuthConfigUpdated,
	type AuthConfigDeletedData,
	AuthConfigDeleted,
} from './events.js';
