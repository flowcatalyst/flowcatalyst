import { pgTable, varchar, bigserial, serial, timestamp, jsonb, text, integer, boolean, smallint, bigint, index, uniqueIndex, foreignKey, primaryKey, unique } from "drizzle-orm/pg-core"
import { sql } from "drizzle-orm"



export const anchorDomains = pgTable("anchor_domains", {
	id: varchar({ length: 17 }).primaryKey(),
	domain: varchar({ length: 255 }).notNull(),
	createdAt: timestamp("created_at", { withTimezone: true }).default(sql`now()`).notNull(),
	updatedAt: timestamp("updated_at", { withTimezone: true }).default(sql`now()`).notNull(),
}, (table) => [
	index("anchor_domains_domain_idx").using("btree", table.domain.asc().nullsLast()),
	unique("anchor_domains_domain_key").on(table.domain),]);

export const applicationClientConfigs = pgTable("application_client_configs", {
	id: varchar({ length: 17 }).primaryKey(),
	createdAt: timestamp("created_at", { withTimezone: true }).default(sql`now()`).notNull(),
	updatedAt: timestamp("updated_at", { withTimezone: true }).default(sql`now()`).notNull(),
	applicationId: varchar("application_id", { length: 17 }).notNull(),
	clientId: varchar("client_id", { length: 17 }).notNull(),
	enabled: boolean().default(true).notNull(),
}, (table) => [
	index("idx_app_client_configs_application").using("btree", table.applicationId.asc().nullsLast()),
	index("idx_app_client_configs_client").using("btree", table.clientId.asc().nullsLast()),
	unique("uq_app_client_configs_app_client").on(table.applicationId, table.clientId),]);

export const applications = pgTable("applications", {
	id: varchar({ length: 17 }).primaryKey(),
	createdAt: timestamp("created_at", { withTimezone: true }).default(sql`now()`).notNull(),
	updatedAt: timestamp("updated_at", { withTimezone: true }).default(sql`now()`).notNull(),
	type: varchar({ length: 50 }).default("APPLICATION").notNull(),
	code: varchar({ length: 50 }).notNull(),
	name: varchar({ length: 255 }).notNull(),
	description: text(),
	iconUrl: varchar("icon_url", { length: 500 }),
	website: varchar({ length: 500 }),
	logo: text(),
	logoMimeType: varchar("logo_mime_type", { length: 100 }),
	defaultBaseUrl: varchar("default_base_url", { length: 500 }),
	serviceAccountId: varchar("service_account_id", { length: 17 }),
	active: boolean().default(true).notNull(),
}, (table) => [
	index("idx_applications_active").using("btree", table.active.asc().nullsLast()),
	index("idx_applications_code").using("btree", table.code.asc().nullsLast()),
	index("idx_applications_type").using("btree", table.type.asc().nullsLast()),
	unique("applications_code_key").on(table.code),]);

export const auditLogs = pgTable("audit_logs", {
	id: varchar({ length: 17 }).primaryKey(),
	entityType: varchar("entity_type", { length: 100 }).notNull(),
	entityId: varchar("entity_id", { length: 17 }).notNull(),
	operation: varchar({ length: 100 }).notNull(),
	operationJson: jsonb("operation_json"),
	principalId: varchar("principal_id", { length: 17 }),
	performedAt: timestamp("performed_at", { withTimezone: true }).default(sql`now()`).notNull(),
}, (table) => [
	index("idx_audit_logs_entity").using("btree", table.entityType.asc().nullsLast(), table.entityId.asc().nullsLast()),
	index("idx_audit_logs_operation").using("btree", table.operation.asc().nullsLast()),
	index("idx_audit_logs_performed").using("btree", table.performedAt.asc().nullsLast()),
	index("idx_audit_logs_principal").using("btree", table.principalId.asc().nullsLast()),
]);

export const authPermissions = pgTable("auth_permissions", {
	id: varchar({ length: 17 }).primaryKey(),
	createdAt: timestamp("created_at", { withTimezone: true }).default(sql`now()`).notNull(),
	updatedAt: timestamp("updated_at", { withTimezone: true }).default(sql`now()`).notNull(),
	code: varchar({ length: 255 }).notNull(),
	subdomain: varchar({ length: 50 }).notNull(),
	context: varchar({ length: 50 }).notNull(),
	aggregate: varchar({ length: 50 }).notNull(),
	action: varchar({ length: 50 }).notNull(),
	description: text(),
}, (table) => [
	index("idx_auth_permissions_code").using("btree", table.code.asc().nullsLast()),
	index("idx_auth_permissions_context").using("btree", table.context.asc().nullsLast()),
	index("idx_auth_permissions_subdomain").using("btree", table.subdomain.asc().nullsLast()),
	unique("auth_permissions_code_key").on(table.code),]);

export const authRoles = pgTable("auth_roles", {
	id: varchar({ length: 17 }).primaryKey(),
	createdAt: timestamp("created_at", { withTimezone: true }).default(sql`now()`).notNull(),
	updatedAt: timestamp("updated_at", { withTimezone: true }).default(sql`now()`).notNull(),
	applicationId: varchar("application_id", { length: 17 }),
	applicationCode: varchar("application_code", { length: 50 }),
	name: varchar({ length: 255 }).notNull(),
	displayName: varchar("display_name", { length: 255 }).notNull(),
	description: text(),
	permissions: jsonb().default([]).notNull(),
	source: varchar({ length: 50 }).default("DATABASE").notNull(),
	clientManaged: boolean("client_managed").default(false).notNull(),
}, (table) => [
	index("idx_auth_roles_application_code").using("btree", table.applicationCode.asc().nullsLast()),
	index("idx_auth_roles_application_id").using("btree", table.applicationId.asc().nullsLast()),
	index("idx_auth_roles_client_managed").using("btree", table.clientManaged.asc().nullsLast()),
	index("idx_auth_roles_name").using("btree", table.name.asc().nullsLast()),
	index("idx_auth_roles_source").using("btree", table.source.asc().nullsLast()),
	unique("auth_roles_name_key").on(table.name),]);

export const clientAccessGrants = pgTable("client_access_grants", {
	id: varchar({ length: 17 }).primaryKey(),
	createdAt: timestamp("created_at", { withTimezone: true }).default(sql`now()`).notNull(),
	updatedAt: timestamp("updated_at", { withTimezone: true }).default(sql`now()`).notNull(),
	principalId: varchar("principal_id", { length: 17 }).notNull(),
	clientId: varchar("client_id", { length: 17 }).notNull(),
	grantedBy: varchar("granted_by", { length: 17 }).notNull(),
	grantedAt: timestamp("granted_at").default(sql`now()`).notNull(),
}, (table) => [
	index("idx_client_access_grants_client").using("btree", table.clientId.asc().nullsLast()),
	index("idx_client_access_grants_principal").using("btree", table.principalId.asc().nullsLast()),
	unique("uq_client_access_grants_principal_client").on(table.principalId, table.clientId),]);

export const clientAuthConfigs = pgTable("client_auth_configs", {
	id: varchar({ length: 17 }).primaryKey(),
	emailDomain: varchar("email_domain", { length: 255 }).notNull(),
	configType: varchar("config_type", { length: 50 }).notNull(),
	primaryClientId: varchar("primary_client_id", { length: 17 }),
	additionalClientIds: jsonb("additional_client_ids").default([]).notNull(),
	grantedClientIds: jsonb("granted_client_ids").default([]).notNull(),
	authProvider: varchar("auth_provider", { length: 50 }).notNull(),
	oidcIssuerUrl: varchar("oidc_issuer_url", { length: 500 }),
	oidcClientId: varchar("oidc_client_id", { length: 255 }),
	oidcMultiTenant: boolean("oidc_multi_tenant").default(false).notNull(),
	oidcIssuerPattern: varchar("oidc_issuer_pattern", { length: 500 }),
	oidcClientSecretRef: varchar("oidc_client_secret_ref", { length: 1000 }),
	createdAt: timestamp("created_at", { withTimezone: true }).default(sql`now()`).notNull(),
	updatedAt: timestamp("updated_at", { withTimezone: true }).default(sql`now()`).notNull(),
}, (table) => [
	index("client_auth_configs_config_type_idx").using("btree", table.configType.asc().nullsLast()),
	index("client_auth_configs_email_domain_idx").using("btree", table.emailDomain.asc().nullsLast()),
	index("client_auth_configs_primary_client_id_idx").using("btree", table.primaryClientId.asc().nullsLast()),
	unique("client_auth_configs_email_domain_key").on(table.emailDomain),]);

export const clients = pgTable("clients", {
	id: varchar({ length: 17 }).primaryKey(),
	createdAt: timestamp("created_at", { withTimezone: true }).default(sql`now()`).notNull(),
	updatedAt: timestamp("updated_at", { withTimezone: true }).default(sql`now()`).notNull(),
	name: varchar({ length: 255 }).notNull(),
	identifier: varchar({ length: 100 }).notNull(),
	status: varchar({ length: 50 }).default("ACTIVE").notNull(),
	statusReason: varchar("status_reason", { length: 255 }),
	statusChangedAt: timestamp("status_changed_at", { withTimezone: true }),
	notes: jsonb().default([]),
}, (table) => [
	index("idx_clients_identifier").using("btree", table.identifier.asc().nullsLast()),
	index("idx_clients_status").using("btree", table.status.asc().nullsLast()),
	unique("clients_identifier_key").on(table.identifier),]);

export const corsAllowedOrigins = pgTable("cors_allowed_origins", {
	id: varchar({ length: 17 }).primaryKey(),
	origin: varchar({ length: 500 }).notNull(),
	description: text(),
	createdBy: varchar("created_by", { length: 17 }),
	createdAt: timestamp("created_at", { withTimezone: true }).default(sql`now()`).notNull(),
	updatedAt: timestamp("updated_at", { withTimezone: true }).default(sql`now()`).notNull(),
}, (table) => [
	index("cors_allowed_origins_origin_idx").using("btree", table.origin.asc().nullsLast()),
	unique("cors_allowed_origins_origin_key").on(table.origin),]);

export const dispatchJobAttempts = pgTable("dispatch_job_attempts", {
	id: varchar({ length: 17 }).primaryKey(),
	dispatchJobId: varchar("dispatch_job_id", { length: 13 }).notNull(),
	attemptNumber: integer("attempt_number"),
	status: varchar({ length: 20 }),
	responseCode: integer("response_code"),
	responseBody: text("response_body"),
	errorMessage: text("error_message"),
	errorStackTrace: text("error_stack_trace"),
	errorType: varchar("error_type", { length: 20 }),
	durationMillis: bigint("duration_millis", { mode: 'number' }),
	attemptedAt: timestamp("attempted_at", { withTimezone: true }),
	completedAt: timestamp("completed_at", { withTimezone: true }),
	createdAt: timestamp("created_at", { withTimezone: true }),
}, (table) => [
	index("idx_dispatch_job_attempts_job").using("btree", table.dispatchJobId.asc().nullsLast()),
	uniqueIndex("idx_dispatch_job_attempts_job_number").using("btree", table.dispatchJobId.asc().nullsLast(), table.attemptNumber.asc().nullsLast()),
]);

export const dispatchJobProjectionFeed = pgTable("dispatch_job_projection_feed", {
	id: bigserial({ mode: 'number' }).primaryKey(),
	dispatchJobId: varchar("dispatch_job_id", { length: 13 }).notNull(),
	operation: varchar({ length: 10 }).notNull(),
	payload: jsonb().notNull(),
	createdAt: timestamp("created_at", { withTimezone: true }).default(sql`now()`).notNull(),
	processed: smallint().default(0).notNull(),
	processedAt: timestamp("processed_at", { withTimezone: true }),
	errorMessage: text("error_message"),
}, (table) => [
	index("idx_dj_projection_feed_in_progress").using("btree", table.id.asc().nullsLast()).where(sql`(processed = 9)`),
	index("idx_dj_projection_feed_processed_at").using("btree", table.processedAt.asc().nullsLast()).where(sql`(processed = 1)`),
	index("idx_dj_projection_feed_unprocessed").using("btree", table.dispatchJobId.asc().nullsLast(), table.id.asc().nullsLast()).where(sql`(processed = 0)`),
]);

export const dispatchJobs = pgTable("dispatch_jobs", {
	id: varchar({ length: 13 }).primaryKey(),
	externalId: varchar("external_id", { length: 100 }),
	source: varchar({ length: 500 }),
	kind: varchar({ length: 20 }).default("EVENT").notNull(),
	code: varchar({ length: 200 }).notNull(),
	subject: varchar({ length: 500 }),
	eventId: varchar("event_id", { length: 13 }),
	correlationId: varchar("correlation_id", { length: 100 }),
	metadata: jsonb().default([]),
	targetUrl: varchar("target_url", { length: 500 }).notNull(),
	protocol: varchar({ length: 30 }).default("HTTP_WEBHOOK").notNull(),
	payload: text(),
	payloadContentType: varchar("payload_content_type", { length: 100 }).default("application/json"),
	dataOnly: boolean("data_only").default(true).notNull(),
	serviceAccountId: varchar("service_account_id", { length: 17 }),
	clientId: varchar("client_id", { length: 17 }),
	subscriptionId: varchar("subscription_id", { length: 17 }),
	mode: varchar({ length: 30 }).default("IMMEDIATE").notNull(),
	dispatchPoolId: varchar("dispatch_pool_id", { length: 17 }),
	messageGroup: varchar("message_group", { length: 200 }),
	sequence: integer().default(99).notNull(),
	timeoutSeconds: integer("timeout_seconds").default(30).notNull(),
	schemaId: varchar("schema_id", { length: 17 }),
	status: varchar({ length: 20 }).default("PENDING").notNull(),
	maxRetries: integer("max_retries").default(3).notNull(),
	retryStrategy: varchar("retry_strategy", { length: 50 }).default("exponential"),
	scheduledFor: timestamp("scheduled_for", { withTimezone: true }),
	expiresAt: timestamp("expires_at", { withTimezone: true }),
	attemptCount: integer("attempt_count").default(0).notNull(),
	lastAttemptAt: timestamp("last_attempt_at", { withTimezone: true }),
	completedAt: timestamp("completed_at", { withTimezone: true }),
	durationMillis: bigint("duration_millis", { mode: 'number' }),
	lastError: text("last_error"),
	idempotencyKey: varchar("idempotency_key", { length: 100 }),
	createdAt: timestamp("created_at", { withTimezone: true }).default(sql`now()`).notNull(),
	updatedAt: timestamp("updated_at", { withTimezone: true }).default(sql`now()`).notNull(),
}, (table) => [
	index("idx_dispatch_jobs_client_id").using("btree", table.clientId.asc().nullsLast()),
	index("idx_dispatch_jobs_created_at").using("btree", table.createdAt.asc().nullsLast()),
	index("idx_dispatch_jobs_message_group").using("btree", table.messageGroup.asc().nullsLast()),
	index("idx_dispatch_jobs_scheduled_for").using("btree", table.scheduledFor.asc().nullsLast()),
	index("idx_dispatch_jobs_status").using("btree", table.status.asc().nullsLast()),
	index("idx_dispatch_jobs_subscription_id").using("btree", table.subscriptionId.asc().nullsLast()),
]);

export const dispatchJobsRead = pgTable("dispatch_jobs_read", {
	id: varchar({ length: 13 }).primaryKey(),
	externalId: varchar("external_id", { length: 100 }),
	source: varchar({ length: 500 }),
	kind: varchar({ length: 20 }).notNull(),
	code: varchar({ length: 200 }).notNull(),
	subject: varchar({ length: 500 }),
	eventId: varchar("event_id", { length: 13 }),
	correlationId: varchar("correlation_id", { length: 100 }),
	targetUrl: varchar("target_url", { length: 500 }).notNull(),
	protocol: varchar({ length: 30 }).notNull(),
	serviceAccountId: varchar("service_account_id", { length: 17 }),
	clientId: varchar("client_id", { length: 17 }),
	subscriptionId: varchar("subscription_id", { length: 17 }),
	dispatchPoolId: varchar("dispatch_pool_id", { length: 17 }),
	mode: varchar({ length: 30 }).notNull(),
	messageGroup: varchar("message_group", { length: 200 }),
	sequence: integer().default(99),
	timeoutSeconds: integer("timeout_seconds").default(30),
	status: varchar({ length: 20 }).notNull(),
	maxRetries: integer("max_retries").notNull(),
	retryStrategy: varchar("retry_strategy", { length: 50 }),
	scheduledFor: timestamp("scheduled_for", { withTimezone: true }),
	expiresAt: timestamp("expires_at", { withTimezone: true }),
	attemptCount: integer("attempt_count").default(0).notNull(),
	lastAttemptAt: timestamp("last_attempt_at", { withTimezone: true }),
	completedAt: timestamp("completed_at", { withTimezone: true }),
	durationMillis: bigint("duration_millis", { mode: 'number' }),
	lastError: text("last_error"),
	idempotencyKey: varchar("idempotency_key", { length: 100 }),
	isCompleted: boolean("is_completed"),
	isTerminal: boolean("is_terminal"),
	application: varchar({ length: 100 }),
	subdomain: varchar({ length: 100 }),
	aggregate: varchar({ length: 100 }),
	createdAt: timestamp("created_at", { withTimezone: true }).notNull(),
	updatedAt: timestamp("updated_at", { withTimezone: true }).notNull(),
	projectedAt: timestamp("projected_at", { withTimezone: true }),
}, (table) => [
	index("idx_dispatch_jobs_read_application").using("btree", table.application.asc().nullsLast()),
	index("idx_dispatch_jobs_read_client_id").using("btree", table.clientId.asc().nullsLast()),
	index("idx_dispatch_jobs_read_created_at").using("btree", table.createdAt.asc().nullsLast()),
	index("idx_dispatch_jobs_read_message_group").using("btree", table.messageGroup.asc().nullsLast()),
	index("idx_dispatch_jobs_read_status").using("btree", table.status.asc().nullsLast()),
	index("idx_dispatch_jobs_read_subscription_id").using("btree", table.subscriptionId.asc().nullsLast()),
]);

export const dispatchPools = pgTable("dispatch_pools", {
	id: varchar({ length: 17 }).primaryKey(),
	createdAt: timestamp("created_at", { withTimezone: true }).default(sql`now()`).notNull(),
	updatedAt: timestamp("updated_at", { withTimezone: true }).default(sql`now()`).notNull(),
	code: varchar({ length: 100 }).notNull(),
	name: varchar({ length: 255 }).notNull(),
	description: varchar({ length: 500 }),
	rateLimit: integer("rate_limit").default(100).notNull(),
	concurrency: integer().default(10).notNull(),
	clientId: varchar("client_id", { length: 17 }),
	clientIdentifier: varchar("client_identifier", { length: 100 }),
	status: varchar({ length: 20 }).default("ACTIVE").notNull(),
}, (table) => [
	index("idx_dispatch_pools_client_id").using("btree", table.clientId.asc().nullsLast()),
	uniqueIndex("idx_dispatch_pools_code_client").using("btree", table.code.asc().nullsLast(), table.clientId.asc().nullsLast()),
	index("idx_dispatch_pools_status").using("btree", table.status.asc().nullsLast()),
]);

export const emailDomainMappingAdditionalClients = pgTable("email_domain_mapping_additional_clients", {
	id: serial().primaryKey(),
	emailDomainMappingId: varchar("email_domain_mapping_id", { length: 17 }).notNull(),
	clientId: varchar("client_id", { length: 17 }).notNull(),
}, (table) => [
	index("idx_edm_additional_clients_mapping").using("btree", table.emailDomainMappingId.asc().nullsLast()),
]);

export const emailDomainMappingAllowedRoles = pgTable("email_domain_mapping_allowed_roles", {
	id: serial().primaryKey(),
	emailDomainMappingId: varchar("email_domain_mapping_id", { length: 17 }).notNull(),
	roleId: varchar("role_id", { length: 17 }).notNull(),
}, (table) => [
	index("idx_edm_allowed_roles_mapping").using("btree", table.emailDomainMappingId.asc().nullsLast()),
]);

export const emailDomainMappingGrantedClients = pgTable("email_domain_mapping_granted_clients", {
	id: serial().primaryKey(),
	emailDomainMappingId: varchar("email_domain_mapping_id", { length: 17 }).notNull(),
	clientId: varchar("client_id", { length: 17 }).notNull(),
}, (table) => [
	index("idx_edm_granted_clients_mapping").using("btree", table.emailDomainMappingId.asc().nullsLast()),
]);

export const emailDomainMappings = pgTable("email_domain_mappings", {
	id: varchar({ length: 17 }).primaryKey(),
	createdAt: timestamp("created_at", { withTimezone: true }).default(sql`now()`).notNull(),
	updatedAt: timestamp("updated_at", { withTimezone: true }).default(sql`now()`).notNull(),
	emailDomain: varchar("email_domain", { length: 255 }).notNull(),
	identityProviderId: varchar("identity_provider_id", { length: 17 }).notNull(),
	scopeType: varchar("scope_type", { length: 20 }).notNull(),
	primaryClientId: varchar("primary_client_id", { length: 17 }),
	requiredOidcTenantId: varchar("required_oidc_tenant_id", { length: 100 }),
	syncRolesFromIdp: boolean("sync_roles_from_idp").default(false).notNull(),
}, (table) => [
	uniqueIndex("idx_email_domain_mappings_domain").using("btree", table.emailDomain.asc().nullsLast()),
	index("idx_email_domain_mappings_idp").using("btree", table.identityProviderId.asc().nullsLast()),
	index("idx_email_domain_mappings_scope").using("btree", table.scopeType.asc().nullsLast()),
]);

export const eventProjectionFeed = pgTable("event_projection_feed", {
	id: bigserial({ mode: 'number' }).primaryKey(),
	eventId: varchar("event_id", { length: 13 }).notNull(),
	payload: jsonb().notNull(),
	createdAt: timestamp("created_at", { withTimezone: true }).default(sql`now()`).notNull(),
	processed: smallint().default(0).notNull(),
	processedAt: timestamp("processed_at", { withTimezone: true }),
	errorMessage: text("error_message"),
}, (table) => [
	index("idx_event_projection_feed_in_progress").using("btree", table.id.asc().nullsLast()).where(sql`(processed = 9)`),
	index("idx_event_projection_feed_unprocessed").using("btree", table.id.asc().nullsLast()).where(sql`(processed = 0)`),
]);

export const eventTypeSpecVersions = pgTable("event_type_spec_versions", {
	id: varchar({ length: 17 }).primaryKey(),
	createdAt: timestamp("created_at", { withTimezone: true }).default(sql`now()`).notNull(),
	updatedAt: timestamp("updated_at", { withTimezone: true }).default(sql`now()`).notNull(),
	eventTypeId: varchar("event_type_id", { length: 17 }).notNull(),
	version: varchar({ length: 20 }).notNull(),
	mimeType: varchar("mime_type", { length: 100 }).notNull(),
	schemaContent: jsonb("schema_content"),
	schemaType: varchar("schema_type", { length: 20 }).notNull(),
	status: varchar({ length: 20 }).default("FINALISING").notNull(),
}, (table) => [
	index("idx_spec_versions_event_type").using("btree", table.eventTypeId.asc().nullsLast()),
	index("idx_spec_versions_status").using("btree", table.status.asc().nullsLast()),
	unique("uq_spec_versions_event_type_version").on(table.eventTypeId, table.version),]);

export const eventTypes = pgTable("event_types", {
	id: varchar({ length: 17 }).primaryKey(),
	createdAt: timestamp("created_at", { withTimezone: true }).default(sql`now()`).notNull(),
	updatedAt: timestamp("updated_at", { withTimezone: true }).default(sql`now()`).notNull(),
	code: varchar({ length: 255 }).notNull(),
	name: varchar({ length: 255 }).notNull(),
	description: text(),
	status: varchar({ length: 20 }).default("CURRENT").notNull(),
	source: varchar({ length: 20 }).default("UI").notNull(),
	clientScoped: boolean("client_scoped").default(false).notNull(),
	application: varchar({ length: 100 }).notNull(),
	subdomain: varchar({ length: 100 }).notNull(),
	aggregate: varchar({ length: 100 }).notNull(),
}, (table) => [
	index("idx_event_types_aggregate").using("btree", table.aggregate.asc().nullsLast()),
	index("idx_event_types_application").using("btree", table.application.asc().nullsLast()),
	index("idx_event_types_code").using("btree", table.code.asc().nullsLast()),
	index("idx_event_types_source").using("btree", table.source.asc().nullsLast()),
	index("idx_event_types_status").using("btree", table.status.asc().nullsLast()),
	index("idx_event_types_subdomain").using("btree", table.subdomain.asc().nullsLast()),
	unique("event_types_code_key").on(table.code),]);

export const events = pgTable("events", {
	id: varchar({ length: 13 }).primaryKey(),
	specVersion: varchar("spec_version", { length: 20 }).default("1.0").notNull(),
	type: varchar({ length: 200 }).notNull(),
	source: varchar({ length: 500 }).notNull(),
	subject: varchar({ length: 500 }),
	time: timestamp({ withTimezone: true }).notNull(),
	data: jsonb(),
	correlationId: varchar("correlation_id", { length: 100 }),
	causationId: varchar("causation_id", { length: 100 }),
	deduplicationId: varchar("deduplication_id", { length: 200 }),
	messageGroup: varchar("message_group", { length: 200 }),
	clientId: varchar("client_id", { length: 17 }),
	contextData: jsonb("context_data"),
	createdAt: timestamp("created_at", { withTimezone: true }).default(sql`now()`).notNull(),
}, (table) => [
	index("idx_events_client_type").using("btree", table.clientId.asc().nullsLast(), table.type.asc().nullsLast()),
	index("idx_events_correlation").using("btree", table.correlationId.asc().nullsLast()),
	uniqueIndex("idx_events_deduplication").using("btree", table.deduplicationId.asc().nullsLast()),
	index("idx_events_time").using("btree", table.time.asc().nullsLast()),
	index("idx_events_type").using("btree", table.type.asc().nullsLast()),
]);

export const eventsRead = pgTable("events_read", {
	id: varchar({ length: 13 }).primaryKey(),
	specVersion: varchar("spec_version", { length: 20 }),
	type: varchar({ length: 200 }).notNull(),
	source: varchar({ length: 500 }).notNull(),
	subject: varchar({ length: 500 }),
	time: timestamp({ withTimezone: true }).notNull(),
	data: text(),
	correlationId: varchar("correlation_id", { length: 100 }),
	causationId: varchar("causation_id", { length: 100 }),
	deduplicationId: varchar("deduplication_id", { length: 200 }),
	messageGroup: varchar("message_group", { length: 200 }),
	clientId: varchar("client_id", { length: 17 }),
	application: varchar({ length: 100 }),
	subdomain: varchar({ length: 100 }),
	aggregate: varchar({ length: 100 }),
	projectedAt: timestamp("projected_at", { withTimezone: true }).default(sql`now()`).notNull(),
}, (table) => [
	index("idx_events_read_aggregate").using("btree", table.aggregate.asc().nullsLast()),
	index("idx_events_read_application").using("btree", table.application.asc().nullsLast()),
	index("idx_events_read_client_id").using("btree", table.clientId.asc().nullsLast()),
	index("idx_events_read_correlation_id").using("btree", table.correlationId.asc().nullsLast()),
	index("idx_events_read_subdomain").using("btree", table.subdomain.asc().nullsLast()),
	index("idx_events_read_time").using("btree", table.time.asc().nullsLast()),
	index("idx_events_read_type").using("btree", table.type.asc().nullsLast()),
]);

export const identityProviderAllowedDomains = pgTable("identity_provider_allowed_domains", {
	id: serial().primaryKey(),
	identityProviderId: varchar("identity_provider_id", { length: 17 }).notNull(),
	emailDomain: varchar("email_domain", { length: 255 }).notNull(),
}, (table) => [
	index("idx_idp_allowed_domains_idp").using("btree", table.identityProviderId.asc().nullsLast()),
]);

export const identityProviders = pgTable("identity_providers", {
	id: varchar({ length: 17 }).primaryKey(),
	createdAt: timestamp("created_at", { withTimezone: true }).default(sql`now()`).notNull(),
	updatedAt: timestamp("updated_at", { withTimezone: true }).default(sql`now()`).notNull(),
	code: varchar({ length: 50 }).notNull(),
	name: varchar({ length: 200 }).notNull(),
	type: varchar({ length: 20 }).notNull(),
	oidcIssuerUrl: varchar("oidc_issuer_url", { length: 500 }),
	oidcClientId: varchar("oidc_client_id", { length: 200 }),
	oidcClientSecretRef: varchar("oidc_client_secret_ref", { length: 500 }),
	oidcMultiTenant: boolean("oidc_multi_tenant").default(false).notNull(),
	oidcIssuerPattern: varchar("oidc_issuer_pattern", { length: 500 }),
}, (table) => [
	uniqueIndex("idx_identity_providers_code").using("btree", table.code.asc().nullsLast()),
]);

export const idpRoleMappings = pgTable("idp_role_mappings", {
	id: varchar({ length: 17 }).primaryKey(),
	createdAt: timestamp("created_at", { withTimezone: true }).default(sql`now()`).notNull(),
	updatedAt: timestamp("updated_at", { withTimezone: true }).default(sql`now()`).notNull(),
	idpRoleName: varchar("idp_role_name", { length: 200 }).notNull(),
	internalRoleName: varchar("internal_role_name", { length: 200 }).notNull(),
}, (table) => [
	uniqueIndex("idx_idp_role_mappings_idp_role_name").using("btree", table.idpRoleName.asc().nullsLast()),
]);

export const oauthClientAllowedOrigins = pgTable("oauth_client_allowed_origins", {
	oauthClientId: varchar("oauth_client_id", { length: 17 }).notNull().references(() => oauthClients.id, { onDelete: "cascade" } ),
	allowedOrigin: varchar("allowed_origin", { length: 200 }).notNull(),
}, (table) => [
	primaryKey({ columns: [table.oauthClientId, table.allowedOrigin], name: "oauth_client_allowed_origins_pkey"}),
	index("idx_oauth_client_allowed_origins_client").using("btree", table.oauthClientId.asc().nullsLast()),
	index("idx_oauth_client_allowed_origins_origin").using("btree", table.allowedOrigin.asc().nullsLast()),
]);

export const oauthClientApplicationIds = pgTable("oauth_client_application_ids", {
	oauthClientId: varchar("oauth_client_id", { length: 17 }).notNull().references(() => oauthClients.id, { onDelete: "cascade" } ),
	applicationId: varchar("application_id", { length: 17 }).notNull(),
}, (table) => [
	primaryKey({ columns: [table.oauthClientId, table.applicationId], name: "oauth_client_application_ids_pkey"}),
	index("idx_oauth_client_application_ids_client").using("btree", table.oauthClientId.asc().nullsLast()),
]);

export const oauthClientGrantTypes = pgTable("oauth_client_grant_types", {
	oauthClientId: varchar("oauth_client_id", { length: 17 }).notNull().references(() => oauthClients.id, { onDelete: "cascade" } ),
	grantType: varchar("grant_type", { length: 50 }).notNull(),
}, (table) => [
	primaryKey({ columns: [table.oauthClientId, table.grantType], name: "oauth_client_grant_types_pkey"}),
	index("idx_oauth_client_grant_types_client").using("btree", table.oauthClientId.asc().nullsLast()),
]);

export const oauthClientRedirectUris = pgTable("oauth_client_redirect_uris", {
	oauthClientId: varchar("oauth_client_id", { length: 17 }).notNull().references(() => oauthClients.id, { onDelete: "cascade" } ),
	redirectUri: varchar("redirect_uri", { length: 500 }).notNull(),
}, (table) => [
	primaryKey({ columns: [table.oauthClientId, table.redirectUri], name: "oauth_client_redirect_uris_pkey"}),
	index("idx_oauth_client_redirect_uris_client").using("btree", table.oauthClientId.asc().nullsLast()),
]);

export const oauthClients = pgTable("oauth_clients", {
	id: varchar({ length: 17 }).primaryKey(),
	clientId: varchar("client_id", { length: 100 }).notNull(),
	clientName: varchar("client_name", { length: 255 }).notNull(),
	clientType: varchar("client_type", { length: 20 }).default("PUBLIC").notNull(),
	clientSecretRef: varchar("client_secret_ref", { length: 500 }),
	defaultScopes: varchar("default_scopes", { length: 500 }),
	pkceRequired: boolean("pkce_required").default(true).notNull(),
	serviceAccountPrincipalId: varchar("service_account_principal_id", { length: 17 }),
	active: boolean().default(true).notNull(),
	createdAt: timestamp("created_at", { withTimezone: true }).default(sql`now()`).notNull(),
	updatedAt: timestamp("updated_at", { withTimezone: true }).default(sql`now()`).notNull(),
}, (table) => [
	index("oauth_clients_active_idx").using("btree", table.active.asc().nullsLast()),
	index("oauth_clients_client_id_idx").using("btree", table.clientId.asc().nullsLast()),
	unique("oauth_clients_client_id_key").on(table.clientId),]);

export const oidcLoginStates = pgTable("oidc_login_states", {
	state: varchar({ length: 200 }).primaryKey(),
	emailDomain: varchar("email_domain", { length: 255 }).notNull(),
	identityProviderId: varchar("identity_provider_id", { length: 17 }).notNull(),
	emailDomainMappingId: varchar("email_domain_mapping_id", { length: 17 }).notNull(),
	nonce: varchar({ length: 200 }).notNull(),
	codeVerifier: varchar("code_verifier", { length: 200 }).notNull(),
	returnUrl: varchar("return_url", { length: 2000 }),
	oauthClientId: varchar("oauth_client_id", { length: 200 }),
	oauthRedirectUri: varchar("oauth_redirect_uri", { length: 2000 }),
	oauthScope: varchar("oauth_scope", { length: 500 }),
	oauthState: varchar("oauth_state", { length: 500 }),
	oauthCodeChallenge: varchar("oauth_code_challenge", { length: 500 }),
	oauthCodeChallengeMethod: varchar("oauth_code_challenge_method", { length: 20 }),
	oauthNonce: varchar("oauth_nonce", { length: 500 }),
	createdAt: timestamp("created_at", { withTimezone: true }).default(sql`now()`).notNull(),
	expiresAt: timestamp("expires_at", { withTimezone: true }).notNull(),
}, (table) => [
	index("idx_oidc_login_states_expires").using("btree", table.expiresAt.asc().nullsLast()),
]);

export const oidcPayloads = pgTable("oidc_payloads", {
	id: varchar({ length: 128 }).primaryKey(),
	type: varchar({ length: 64 }).notNull(),
	payload: jsonb().notNull(),
	grantId: varchar("grant_id", { length: 128 }),
	userCode: varchar("user_code", { length: 128 }),
	uid: varchar({ length: 128 }),
	expiresAt: timestamp("expires_at", { withTimezone: true }),
	consumedAt: timestamp("consumed_at", { withTimezone: true }),
	createdAt: timestamp("created_at", { withTimezone: true }).default(sql`now()`).notNull(),
}, (table) => [
	index("oidc_payloads_expires_at_idx").using("btree", table.expiresAt.asc().nullsLast()),
	index("oidc_payloads_grant_id_idx").using("btree", table.grantId.asc().nullsLast()),
	index("oidc_payloads_type_idx").using("btree", table.type.asc().nullsLast()),
	index("oidc_payloads_uid_idx").using("btree", table.uid.asc().nullsLast()),
	index("oidc_payloads_user_code_idx").using("btree", table.userCode.asc().nullsLast()),
]);

export const platformConfigAccess = pgTable("platform_config_access", {
	id: varchar({ length: 17 }).primaryKey(),
	applicationCode: varchar("application_code", { length: 100 }).notNull(),
	roleCode: varchar("role_code", { length: 200 }).notNull(),
	canRead: boolean("can_read").default(true).notNull(),
	canWrite: boolean("can_write").default(false).notNull(),
	createdAt: timestamp("created_at", { withTimezone: true }).default(sql`now()`).notNull(),
}, (table) => [
	index("idx_config_access_app").using("btree", table.applicationCode.asc().nullsLast()),
	index("idx_config_access_role").using("btree", table.roleCode.asc().nullsLast()),
	unique("uq_config_access_role").on(table.applicationCode, table.roleCode),]);

export const platformConfigs = pgTable("platform_configs", {
	id: varchar({ length: 17 }).primaryKey(),
	applicationCode: varchar("application_code", { length: 100 }).notNull(),
	section: varchar({ length: 100 }).notNull(),
	property: varchar({ length: 100 }).notNull(),
	scope: varchar({ length: 20 }).notNull(),
	clientId: varchar("client_id", { length: 17 }),
	valueType: varchar("value_type", { length: 20 }).notNull(),
	value: text().notNull(),
	description: text(),
	createdAt: timestamp("created_at", { withTimezone: true }).default(sql`now()`).notNull(),
	updatedAt: timestamp("updated_at", { withTimezone: true }).default(sql`now()`).notNull(),
}, (table) => [
	index("idx_platform_configs_app_section").using("btree", table.applicationCode.asc().nullsLast(), table.section.asc().nullsLast()),
	index("idx_platform_configs_lookup").using("btree", table.applicationCode.asc().nullsLast(), table.section.asc().nullsLast(), table.scope.asc().nullsLast(), table.clientId.asc().nullsLast()),
	unique("uq_platform_config_key").on(table.applicationCode, table.section, table.property, table.scope, table.clientId),]);

export const principalApplicationAccess = pgTable("principal_application_access", {
	principalId: varchar("principal_id", { length: 17 }).notNull(),
	applicationId: varchar("application_id", { length: 17 }).notNull(),
	grantedAt: timestamp("granted_at", { withTimezone: true }).default(sql`now()`).notNull(),
}, (table) => [
	primaryKey({ columns: [table.principalId, table.applicationId], name: "principal_application_access_pkey"}),
	index("idx_principal_app_access_app_id").using("btree", table.applicationId.asc().nullsLast()),
]);

export const principalRoles = pgTable("principal_roles", {
	principalId: varchar("principal_id", { length: 17 }).notNull().references(() => principals.id, { onDelete: "cascade" } ),
	roleName: varchar("role_name", { length: 100 }).notNull(),
	assignmentSource: varchar("assignment_source", { length: 50 }),
	assignedAt: timestamp("assigned_at", { withTimezone: true }).default(sql`now()`).notNull(),
}, (table) => [
	primaryKey({ columns: [table.principalId, table.roleName], name: "principal_roles_pkey"}),
	index("idx_principal_roles_assigned_at").using("btree", table.assignedAt.asc().nullsLast()),
	index("idx_principal_roles_role_name").using("btree", table.roleName.asc().nullsLast()),
]);

export const principals = pgTable("principals", {
	id: varchar({ length: 17 }).primaryKey(),
	createdAt: timestamp("created_at", { withTimezone: true }).default(sql`now()`).notNull(),
	updatedAt: timestamp("updated_at", { withTimezone: true }).default(sql`now()`).notNull(),
	type: varchar({ length: 20 }).notNull(),
	scope: varchar({ length: 20 }),
	clientId: varchar("client_id", { length: 17 }),
	applicationId: varchar("application_id", { length: 17 }),
	name: varchar({ length: 255 }).notNull(),
	active: boolean().default(true).notNull(),
	email: varchar({ length: 255 }),
	emailDomain: varchar("email_domain", { length: 100 }),
	idpType: varchar("idp_type", { length: 50 }),
	externalIdpId: varchar("external_idp_id", { length: 255 }),
	passwordHash: varchar("password_hash", { length: 255 }),
	lastLoginAt: timestamp("last_login_at", { withTimezone: true }),
	serviceAccountId: varchar("service_account_id", { length: 17 }),
}, (table) => [
	index("idx_principals_active").using("btree", table.active.asc().nullsLast()),
	index("idx_principals_client_id").using("btree", table.clientId.asc().nullsLast()),
	uniqueIndex("idx_principals_email").using("btree", table.email.asc().nullsLast()),
	index("idx_principals_email_domain").using("btree", table.emailDomain.asc().nullsLast()),
	uniqueIndex("idx_principals_service_account_id").using("btree", table.serviceAccountId.asc().nullsLast()),
	index("idx_principals_type").using("btree", table.type.asc().nullsLast()),
]);

export const serviceAccounts = pgTable("service_accounts", {
	id: varchar({ length: 17 }).primaryKey(),
	code: varchar({ length: 100 }).notNull(),
	name: varchar({ length: 200 }).notNull(),
	description: varchar({ length: 500 }),
	applicationId: varchar("application_id", { length: 17 }),
	active: boolean().default(true).notNull(),
	whAuthType: varchar("wh_auth_type", { length: 50 }),
	whAuthTokenRef: varchar("wh_auth_token_ref", { length: 500 }),
	whSigningSecretRef: varchar("wh_signing_secret_ref", { length: 500 }),
	whSigningAlgorithm: varchar("wh_signing_algorithm", { length: 50 }),
	whCredentialsCreatedAt: timestamp("wh_credentials_created_at", { withTimezone: true }),
	whCredentialsRegeneratedAt: timestamp("wh_credentials_regenerated_at", { withTimezone: true }),
	lastUsedAt: timestamp("last_used_at", { withTimezone: true }),
	createdAt: timestamp("created_at", { withTimezone: true }).default(sql`now()`).notNull(),
	updatedAt: timestamp("updated_at", { withTimezone: true }).default(sql`now()`).notNull(),
}, (table) => [
	index("idx_service_accounts_active").using("btree", table.active.asc().nullsLast()),
	index("idx_service_accounts_application_id").using("btree", table.applicationId.asc().nullsLast()),
	uniqueIndex("idx_service_accounts_code").using("btree", table.code.asc().nullsLast()),
]);

export const subscriptionCustomConfigs = pgTable("subscription_custom_configs", {
	id: serial().primaryKey(),
	subscriptionId: varchar("subscription_id", { length: 17 }).notNull(),
	configKey: varchar("config_key", { length: 100 }).notNull(),
	configValue: varchar("config_value", { length: 1000 }).notNull(),
}, (table) => [
	index("idx_sub_configs_subscription").using("btree", table.subscriptionId.asc().nullsLast()),
]);

export const subscriptionEventTypes = pgTable("subscription_event_types", {
	id: serial().primaryKey(),
	subscriptionId: varchar("subscription_id", { length: 17 }).notNull(),
	eventTypeId: varchar("event_type_id", { length: 17 }),
	eventTypeCode: varchar("event_type_code", { length: 255 }).notNull(),
	specVersion: varchar("spec_version", { length: 50 }),
}, (table) => [
	index("idx_sub_event_types_event_type").using("btree", table.eventTypeId.asc().nullsLast()),
	index("idx_sub_event_types_subscription").using("btree", table.subscriptionId.asc().nullsLast()),
]);

export const subscriptions = pgTable("subscriptions", {
	id: varchar({ length: 17 }).primaryKey(),
	createdAt: timestamp("created_at", { withTimezone: true }).default(sql`now()`).notNull(),
	updatedAt: timestamp("updated_at", { withTimezone: true }).default(sql`now()`).notNull(),
	code: varchar({ length: 100 }).notNull(),
	applicationCode: varchar("application_code", { length: 100 }),
	name: varchar({ length: 255 }).notNull(),
	description: text(),
	clientId: varchar("client_id", { length: 17 }),
	clientIdentifier: varchar("client_identifier", { length: 100 }),
	clientScoped: boolean("client_scoped").default(false).notNull(),
	target: varchar({ length: 500 }).notNull(),
	queue: varchar({ length: 255 }),
	source: varchar({ length: 20 }).default("UI").notNull(),
	status: varchar({ length: 20 }).default("ACTIVE").notNull(),
	maxAgeSeconds: integer("max_age_seconds").default(86400).notNull(),
	dispatchPoolId: varchar("dispatch_pool_id", { length: 17 }),
	dispatchPoolCode: varchar("dispatch_pool_code", { length: 100 }),
	delaySeconds: integer("delay_seconds").default(0).notNull(),
	sequence: integer().default(99).notNull(),
	mode: varchar({ length: 20 }).default("IMMEDIATE").notNull(),
	timeoutSeconds: integer("timeout_seconds").default(30).notNull(),
	maxRetries: integer("max_retries").default(3).notNull(),
	serviceAccountId: varchar("service_account_id", { length: 17 }),
	dataOnly: boolean("data_only").default(true).notNull(),
}, (table) => [
	index("idx_subscriptions_client_id").using("btree", table.clientId.asc().nullsLast()),
	uniqueIndex("idx_subscriptions_code_client").using("btree", table.code.asc().nullsLast(), table.clientId.asc().nullsLast()),
	index("idx_subscriptions_dispatch_pool").using("btree", table.dispatchPoolId.asc().nullsLast()),
	index("idx_subscriptions_source").using("btree", table.source.asc().nullsLast()),
	index("idx_subscriptions_status").using("btree", table.status.asc().nullsLast()),
]);
