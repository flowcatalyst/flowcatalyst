/**
 * Email Domain Mapping Domain Exports
 */

export type { ScopeType } from "./scope-type.js";
export {
	type EmailDomainMapping,
	type NewEmailDomainMapping,
	createEmailDomainMapping,
	updateEmailDomainMapping,
	getMappingAccessibleClientIds,
	hasRoleRestrictions,
} from "./email-domain-mapping.js";
export {
	EmailDomainMappingCreated,
	EmailDomainMappingUpdated,
	EmailDomainMappingDeleted,
} from "./events.js";
