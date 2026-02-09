CREATE TABLE "anchor_domains" (
	"id" varchar(17) PRIMARY KEY,
	"domain" varchar(255) NOT NULL UNIQUE,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "application_client_configs" (
	"id" varchar(17) PRIMARY KEY,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"application_id" varchar(17) NOT NULL,
	"client_id" varchar(17) NOT NULL,
	"enabled" boolean DEFAULT true NOT NULL,
	CONSTRAINT "uq_app_client_configs_app_client" UNIQUE("application_id","client_id")
);
--> statement-breakpoint
CREATE TABLE "applications" (
	"id" varchar(17) PRIMARY KEY,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"type" varchar(50) DEFAULT 'APPLICATION' NOT NULL,
	"code" varchar(50) NOT NULL UNIQUE,
	"name" varchar(255) NOT NULL,
	"description" text,
	"icon_url" varchar(500),
	"website" varchar(500),
	"logo" text,
	"logo_mime_type" varchar(100),
	"default_base_url" varchar(500),
	"service_account_id" varchar(17),
	"active" boolean DEFAULT true NOT NULL
);
--> statement-breakpoint
CREATE TABLE "audit_logs" (
	"id" varchar(17) PRIMARY KEY,
	"entity_type" varchar(100) NOT NULL,
	"entity_id" varchar(17) NOT NULL,
	"operation" varchar(100) NOT NULL,
	"operation_json" jsonb,
	"principal_id" varchar(17),
	"performed_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "auth_permissions" (
	"id" varchar(17) PRIMARY KEY,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"code" varchar(255) NOT NULL UNIQUE,
	"subdomain" varchar(50) NOT NULL,
	"context" varchar(50) NOT NULL,
	"aggregate" varchar(50) NOT NULL,
	"action" varchar(50) NOT NULL,
	"description" text
);
--> statement-breakpoint
CREATE TABLE "auth_roles" (
	"id" varchar(17) PRIMARY KEY,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"application_id" varchar(17),
	"application_code" varchar(50),
	"name" varchar(255) NOT NULL UNIQUE,
	"display_name" varchar(255) NOT NULL,
	"description" text,
	"permissions" jsonb DEFAULT '[]' NOT NULL,
	"source" varchar(50) DEFAULT 'DATABASE' NOT NULL,
	"client_managed" boolean DEFAULT false NOT NULL
);
--> statement-breakpoint
CREATE TABLE "client_access_grants" (
	"id" varchar(17) PRIMARY KEY,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"principal_id" varchar(17) NOT NULL,
	"client_id" varchar(17) NOT NULL,
	"granted_by" varchar(17) NOT NULL,
	"granted_at" timestamp DEFAULT now() NOT NULL,
	CONSTRAINT "uq_client_access_grants_principal_client" UNIQUE("principal_id","client_id")
);
--> statement-breakpoint
CREATE TABLE "client_auth_configs" (
	"id" varchar(17) PRIMARY KEY,
	"email_domain" varchar(255) NOT NULL UNIQUE,
	"config_type" varchar(50) NOT NULL,
	"primary_client_id" varchar(17),
	"additional_client_ids" jsonb DEFAULT '[]' NOT NULL,
	"granted_client_ids" jsonb DEFAULT '[]' NOT NULL,
	"auth_provider" varchar(50) NOT NULL,
	"oidc_issuer_url" varchar(500),
	"oidc_client_id" varchar(255),
	"oidc_multi_tenant" boolean DEFAULT false NOT NULL,
	"oidc_issuer_pattern" varchar(500),
	"oidc_client_secret_ref" varchar(1000),
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "clients" (
	"id" varchar(17) PRIMARY KEY,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"name" varchar(255) NOT NULL,
	"identifier" varchar(100) NOT NULL UNIQUE,
	"status" varchar(50) DEFAULT 'ACTIVE' NOT NULL,
	"status_reason" varchar(255),
	"status_changed_at" timestamp with time zone,
	"notes" jsonb DEFAULT '[]'
);
--> statement-breakpoint
CREATE TABLE "cors_allowed_origins" (
	"id" varchar(17) PRIMARY KEY,
	"origin" varchar(500) NOT NULL UNIQUE,
	"description" text,
	"created_by" varchar(17),
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "dispatch_job_attempts" (
	"id" varchar(17) PRIMARY KEY,
	"dispatch_job_id" varchar(13) NOT NULL,
	"attempt_number" integer,
	"status" varchar(20),
	"response_code" integer,
	"response_body" text,
	"error_message" text,
	"error_stack_trace" text,
	"error_type" varchar(20),
	"duration_millis" bigint,
	"attempted_at" timestamp with time zone,
	"completed_at" timestamp with time zone,
	"created_at" timestamp with time zone
);
--> statement-breakpoint
CREATE TABLE "dispatch_job_projection_feed" (
	"id" bigserial PRIMARY KEY,
	"dispatch_job_id" varchar(13) NOT NULL,
	"operation" varchar(10) NOT NULL,
	"payload" jsonb NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"processed" smallint DEFAULT 0 NOT NULL,
	"processed_at" timestamp with time zone,
	"error_message" text
);
--> statement-breakpoint
CREATE TABLE "dispatch_jobs_read" (
	"id" varchar(13) PRIMARY KEY,
	"external_id" varchar(100),
	"source" varchar(500),
	"kind" varchar(20) NOT NULL,
	"code" varchar(200) NOT NULL,
	"subject" varchar(500),
	"event_id" varchar(13),
	"correlation_id" varchar(100),
	"target_url" varchar(500) NOT NULL,
	"protocol" varchar(30) NOT NULL,
	"service_account_id" varchar(17),
	"client_id" varchar(17),
	"subscription_id" varchar(17),
	"dispatch_pool_id" varchar(17),
	"mode" varchar(30) NOT NULL,
	"message_group" varchar(200),
	"sequence" integer DEFAULT 99,
	"timeout_seconds" integer DEFAULT 30,
	"status" varchar(20) NOT NULL,
	"max_retries" integer NOT NULL,
	"retry_strategy" varchar(50),
	"scheduled_for" timestamp with time zone,
	"expires_at" timestamp with time zone,
	"attempt_count" integer DEFAULT 0 NOT NULL,
	"last_attempt_at" timestamp with time zone,
	"completed_at" timestamp with time zone,
	"duration_millis" bigint,
	"last_error" text,
	"idempotency_key" varchar(100),
	"is_completed" boolean,
	"is_terminal" boolean,
	"application" varchar(100),
	"subdomain" varchar(100),
	"aggregate" varchar(100),
	"created_at" timestamp with time zone NOT NULL,
	"updated_at" timestamp with time zone NOT NULL,
	"projected_at" timestamp with time zone
);
--> statement-breakpoint
CREATE TABLE "dispatch_pools" (
	"id" varchar(17) PRIMARY KEY,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"code" varchar(100) NOT NULL,
	"name" varchar(255) NOT NULL,
	"description" varchar(500),
	"rate_limit" integer DEFAULT 100 NOT NULL,
	"concurrency" integer DEFAULT 10 NOT NULL,
	"client_id" varchar(17),
	"client_identifier" varchar(100),
	"status" varchar(20) DEFAULT 'ACTIVE' NOT NULL
);
--> statement-breakpoint
CREATE TABLE "email_domain_mapping_additional_clients" (
	"id" serial PRIMARY KEY,
	"email_domain_mapping_id" varchar(17) NOT NULL,
	"client_id" varchar(17) NOT NULL
);
--> statement-breakpoint
CREATE TABLE "email_domain_mapping_allowed_roles" (
	"id" serial PRIMARY KEY,
	"email_domain_mapping_id" varchar(17) NOT NULL,
	"role_id" varchar(17) NOT NULL
);
--> statement-breakpoint
CREATE TABLE "email_domain_mapping_granted_clients" (
	"id" serial PRIMARY KEY,
	"email_domain_mapping_id" varchar(17) NOT NULL,
	"client_id" varchar(17) NOT NULL
);
--> statement-breakpoint
CREATE TABLE "email_domain_mappings" (
	"id" varchar(17) PRIMARY KEY,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"email_domain" varchar(255) NOT NULL,
	"identity_provider_id" varchar(17) NOT NULL,
	"scope_type" varchar(20) NOT NULL,
	"primary_client_id" varchar(17),
	"required_oidc_tenant_id" varchar(100),
	"sync_roles_from_idp" boolean DEFAULT false NOT NULL
);
--> statement-breakpoint
CREATE TABLE "event_projection_feed" (
	"id" bigserial PRIMARY KEY,
	"event_id" varchar(13) NOT NULL,
	"payload" jsonb NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"processed" smallint DEFAULT 0 NOT NULL,
	"processed_at" timestamp with time zone,
	"error_message" text
);
--> statement-breakpoint
CREATE TABLE "event_type_spec_versions" (
	"id" varchar(17) PRIMARY KEY,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"event_type_id" varchar(17) NOT NULL,
	"version" varchar(20) NOT NULL,
	"mime_type" varchar(100) NOT NULL,
	"schema_content" jsonb,
	"schema_type" varchar(20) NOT NULL,
	"status" varchar(20) DEFAULT 'FINALISING' NOT NULL,
	CONSTRAINT "uq_spec_versions_event_type_version" UNIQUE("event_type_id","version")
);
--> statement-breakpoint
CREATE TABLE "event_types" (
	"id" varchar(17) PRIMARY KEY,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"code" varchar(255) NOT NULL UNIQUE,
	"name" varchar(255) NOT NULL,
	"description" text,
	"status" varchar(20) DEFAULT 'CURRENT' NOT NULL,
	"source" varchar(20) DEFAULT 'UI' NOT NULL,
	"client_scoped" boolean DEFAULT false NOT NULL,
	"application" varchar(100) NOT NULL,
	"subdomain" varchar(100) NOT NULL,
	"aggregate" varchar(100) NOT NULL
);
--> statement-breakpoint
CREATE TABLE "events" (
	"id" varchar(13) PRIMARY KEY,
	"spec_version" varchar(20) DEFAULT '1.0' NOT NULL,
	"type" varchar(200) NOT NULL,
	"source" varchar(500) NOT NULL,
	"subject" varchar(500),
	"time" timestamp with time zone NOT NULL,
	"data" jsonb,
	"correlation_id" varchar(100),
	"causation_id" varchar(100),
	"deduplication_id" varchar(200),
	"message_group" varchar(200),
	"client_id" varchar(17),
	"context_data" jsonb,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "events_read" (
	"id" varchar(13) PRIMARY KEY,
	"spec_version" varchar(20),
	"type" varchar(200) NOT NULL,
	"source" varchar(500) NOT NULL,
	"subject" varchar(500),
	"time" timestamp with time zone NOT NULL,
	"data" text,
	"correlation_id" varchar(100),
	"causation_id" varchar(100),
	"deduplication_id" varchar(200),
	"message_group" varchar(200),
	"client_id" varchar(17),
	"application" varchar(100),
	"subdomain" varchar(100),
	"aggregate" varchar(100),
	"projected_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "identity_provider_allowed_domains" (
	"id" serial PRIMARY KEY,
	"identity_provider_id" varchar(17) NOT NULL,
	"email_domain" varchar(255) NOT NULL
);
--> statement-breakpoint
CREATE TABLE "identity_providers" (
	"id" varchar(17) PRIMARY KEY,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"code" varchar(50) NOT NULL,
	"name" varchar(200) NOT NULL,
	"type" varchar(20) NOT NULL,
	"oidc_issuer_url" varchar(500),
	"oidc_client_id" varchar(200),
	"oidc_client_secret_ref" varchar(500),
	"oidc_multi_tenant" boolean DEFAULT false NOT NULL,
	"oidc_issuer_pattern" varchar(500)
);
--> statement-breakpoint
CREATE TABLE "idp_role_mappings" (
	"id" varchar(17) PRIMARY KEY,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"idp_role_name" varchar(200) NOT NULL,
	"internal_role_name" varchar(200) NOT NULL
);
--> statement-breakpoint
CREATE TABLE "oauth_client_allowed_origins" (
	"oauth_client_id" varchar(17),
	"allowed_origin" varchar(200),
	CONSTRAINT "oauth_client_allowed_origins_pkey" PRIMARY KEY("oauth_client_id","allowed_origin")
);
--> statement-breakpoint
CREATE TABLE "oauth_client_application_ids" (
	"oauth_client_id" varchar(17),
	"application_id" varchar(17),
	CONSTRAINT "oauth_client_application_ids_pkey" PRIMARY KEY("oauth_client_id","application_id")
);
--> statement-breakpoint
CREATE TABLE "oauth_client_grant_types" (
	"oauth_client_id" varchar(17),
	"grant_type" varchar(50),
	CONSTRAINT "oauth_client_grant_types_pkey" PRIMARY KEY("oauth_client_id","grant_type")
);
--> statement-breakpoint
CREATE TABLE "oauth_client_redirect_uris" (
	"oauth_client_id" varchar(17),
	"redirect_uri" varchar(500),
	CONSTRAINT "oauth_client_redirect_uris_pkey" PRIMARY KEY("oauth_client_id","redirect_uri")
);
--> statement-breakpoint
CREATE TABLE "oauth_clients" (
	"id" varchar(17) PRIMARY KEY,
	"client_id" varchar(100) NOT NULL UNIQUE,
	"client_name" varchar(255) NOT NULL,
	"client_type" varchar(20) DEFAULT 'PUBLIC' NOT NULL,
	"client_secret_ref" varchar(500),
	"default_scopes" varchar(500),
	"pkce_required" boolean DEFAULT true NOT NULL,
	"service_account_principal_id" varchar(17),
	"active" boolean DEFAULT true NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "oidc_login_states" (
	"state" varchar(200) PRIMARY KEY,
	"email_domain" varchar(255) NOT NULL,
	"identity_provider_id" varchar(17) NOT NULL,
	"email_domain_mapping_id" varchar(17) NOT NULL,
	"nonce" varchar(200) NOT NULL,
	"code_verifier" varchar(200) NOT NULL,
	"return_url" varchar(2000),
	"oauth_client_id" varchar(200),
	"oauth_redirect_uri" varchar(2000),
	"oauth_scope" varchar(500),
	"oauth_state" varchar(500),
	"oauth_code_challenge" varchar(500),
	"oauth_code_challenge_method" varchar(20),
	"oauth_nonce" varchar(500),
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"expires_at" timestamp with time zone NOT NULL
);
--> statement-breakpoint
CREATE TABLE "oidc_payloads" (
	"id" varchar(128) PRIMARY KEY,
	"type" varchar(64) NOT NULL,
	"payload" jsonb NOT NULL,
	"grant_id" varchar(128),
	"user_code" varchar(128),
	"uid" varchar(128),
	"expires_at" timestamp with time zone,
	"consumed_at" timestamp with time zone,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "platform_config_access" (
	"id" varchar(17) PRIMARY KEY,
	"application_code" varchar(100) NOT NULL,
	"role_code" varchar(200) NOT NULL,
	"can_read" boolean DEFAULT true NOT NULL,
	"can_write" boolean DEFAULT false NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "uq_config_access_role" UNIQUE("application_code","role_code")
);
--> statement-breakpoint
CREATE TABLE "platform_configs" (
	"id" varchar(17) PRIMARY KEY,
	"application_code" varchar(100) NOT NULL,
	"section" varchar(100) NOT NULL,
	"property" varchar(100) NOT NULL,
	"scope" varchar(20) NOT NULL,
	"client_id" varchar(17),
	"value_type" varchar(20) NOT NULL,
	"value" text NOT NULL,
	"description" text,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "uq_platform_config_key" UNIQUE("application_code","section","property","scope","client_id")
);
--> statement-breakpoint
CREATE TABLE "principal_application_access" (
	"principal_id" varchar(17),
	"application_id" varchar(17),
	"granted_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "principal_application_access_pkey" PRIMARY KEY("principal_id","application_id")
);
--> statement-breakpoint
CREATE TABLE "principal_roles" (
	"principal_id" varchar(17),
	"role_name" varchar(100),
	"assignment_source" varchar(50),
	"assigned_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "principal_roles_pkey" PRIMARY KEY("principal_id","role_name")
);
--> statement-breakpoint
CREATE TABLE "principals" (
	"id" varchar(17) PRIMARY KEY,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"type" varchar(20) NOT NULL,
	"scope" varchar(20),
	"client_id" varchar(17),
	"application_id" varchar(17),
	"name" varchar(255) NOT NULL,
	"active" boolean DEFAULT true NOT NULL,
	"email" varchar(255),
	"email_domain" varchar(100),
	"idp_type" varchar(50),
	"external_idp_id" varchar(255),
	"password_hash" varchar(255),
	"last_login_at" timestamp with time zone,
	"service_account_id" varchar(17)
);
--> statement-breakpoint
CREATE TABLE "service_accounts" (
	"id" varchar(17) PRIMARY KEY,
	"code" varchar(100) NOT NULL,
	"name" varchar(200) NOT NULL,
	"description" varchar(500),
	"application_id" varchar(17),
	"active" boolean DEFAULT true NOT NULL,
	"wh_auth_type" varchar(50),
	"wh_auth_token_ref" varchar(500),
	"wh_signing_secret_ref" varchar(500),
	"wh_signing_algorithm" varchar(50),
	"wh_credentials_created_at" timestamp with time zone,
	"wh_credentials_regenerated_at" timestamp with time zone,
	"last_used_at" timestamp with time zone,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "subscription_custom_configs" (
	"id" serial PRIMARY KEY,
	"subscription_id" varchar(17) NOT NULL,
	"config_key" varchar(100) NOT NULL,
	"config_value" varchar(1000) NOT NULL
);
--> statement-breakpoint
CREATE TABLE "subscription_event_types" (
	"id" serial PRIMARY KEY,
	"subscription_id" varchar(17) NOT NULL,
	"event_type_id" varchar(17),
	"event_type_code" varchar(255) NOT NULL,
	"spec_version" varchar(50)
);
--> statement-breakpoint
CREATE TABLE "subscriptions" (
	"id" varchar(17) PRIMARY KEY,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"code" varchar(100) NOT NULL,
	"application_code" varchar(100),
	"name" varchar(255) NOT NULL,
	"description" text,
	"client_id" varchar(17),
	"client_identifier" varchar(100),
	"client_scoped" boolean DEFAULT false NOT NULL,
	"target" varchar(500) NOT NULL,
	"queue" varchar(255),
	"source" varchar(20) DEFAULT 'UI' NOT NULL,
	"status" varchar(20) DEFAULT 'ACTIVE' NOT NULL,
	"max_age_seconds" integer DEFAULT 86400 NOT NULL,
	"dispatch_pool_id" varchar(17),
	"dispatch_pool_code" varchar(100),
	"delay_seconds" integer DEFAULT 0 NOT NULL,
	"sequence" integer DEFAULT 99 NOT NULL,
	"mode" varchar(20) DEFAULT 'IMMEDIATE' NOT NULL,
	"timeout_seconds" integer DEFAULT 30 NOT NULL,
	"max_retries" integer DEFAULT 3 NOT NULL,
	"service_account_id" varchar(17),
	"data_only" boolean DEFAULT true NOT NULL
);
--> statement-breakpoint
CREATE INDEX "anchor_domains_domain_idx" ON "anchor_domains" ("domain");--> statement-breakpoint
CREATE INDEX "idx_app_client_configs_application" ON "application_client_configs" ("application_id");--> statement-breakpoint
CREATE INDEX "idx_app_client_configs_client" ON "application_client_configs" ("client_id");--> statement-breakpoint
CREATE INDEX "idx_applications_code" ON "applications" ("code");--> statement-breakpoint
CREATE INDEX "idx_applications_type" ON "applications" ("type");--> statement-breakpoint
CREATE INDEX "idx_applications_active" ON "applications" ("active");--> statement-breakpoint
CREATE INDEX "idx_audit_logs_entity" ON "audit_logs" ("entity_type","entity_id");--> statement-breakpoint
CREATE INDEX "idx_audit_logs_performed" ON "audit_logs" ("performed_at");--> statement-breakpoint
CREATE INDEX "idx_audit_logs_principal" ON "audit_logs" ("principal_id");--> statement-breakpoint
CREATE INDEX "idx_audit_logs_operation" ON "audit_logs" ("operation");--> statement-breakpoint
CREATE INDEX "idx_auth_permissions_code" ON "auth_permissions" ("code");--> statement-breakpoint
CREATE INDEX "idx_auth_permissions_subdomain" ON "auth_permissions" ("subdomain");--> statement-breakpoint
CREATE INDEX "idx_auth_permissions_context" ON "auth_permissions" ("context");--> statement-breakpoint
CREATE INDEX "idx_auth_roles_name" ON "auth_roles" ("name");--> statement-breakpoint
CREATE INDEX "idx_auth_roles_application_id" ON "auth_roles" ("application_id");--> statement-breakpoint
CREATE INDEX "idx_auth_roles_application_code" ON "auth_roles" ("application_code");--> statement-breakpoint
CREATE INDEX "idx_auth_roles_source" ON "auth_roles" ("source");--> statement-breakpoint
CREATE INDEX "idx_auth_roles_client_managed" ON "auth_roles" ("client_managed");--> statement-breakpoint
CREATE INDEX "idx_client_access_grants_principal" ON "client_access_grants" ("principal_id");--> statement-breakpoint
CREATE INDEX "idx_client_access_grants_client" ON "client_access_grants" ("client_id");--> statement-breakpoint
CREATE INDEX "client_auth_configs_email_domain_idx" ON "client_auth_configs" ("email_domain");--> statement-breakpoint
CREATE INDEX "client_auth_configs_config_type_idx" ON "client_auth_configs" ("config_type");--> statement-breakpoint
CREATE INDEX "client_auth_configs_primary_client_id_idx" ON "client_auth_configs" ("primary_client_id");--> statement-breakpoint
CREATE INDEX "idx_clients_identifier" ON "clients" ("identifier");--> statement-breakpoint
CREATE INDEX "idx_clients_status" ON "clients" ("status");--> statement-breakpoint
CREATE INDEX "cors_allowed_origins_origin_idx" ON "cors_allowed_origins" ("origin");--> statement-breakpoint
CREATE UNIQUE INDEX "idx_dispatch_job_attempts_job_number" ON "dispatch_job_attempts" ("dispatch_job_id","attempt_number");--> statement-breakpoint
CREATE INDEX "idx_dispatch_job_attempts_job" ON "dispatch_job_attempts" ("dispatch_job_id");--> statement-breakpoint
CREATE INDEX "idx_dj_projection_feed_unprocessed" ON "dispatch_job_projection_feed" ("dispatch_job_id","id") WHERE "processed" = 0;--> statement-breakpoint
CREATE INDEX "idx_dj_projection_feed_in_progress" ON "dispatch_job_projection_feed" ("id") WHERE "processed" = 9;--> statement-breakpoint
CREATE INDEX "idx_dj_projection_feed_processed_at" ON "dispatch_job_projection_feed" ("processed_at") WHERE "processed" = 1;--> statement-breakpoint
CREATE INDEX "idx_dispatch_jobs_read_status" ON "dispatch_jobs_read" ("status");--> statement-breakpoint
CREATE INDEX "idx_dispatch_jobs_read_client_id" ON "dispatch_jobs_read" ("client_id");--> statement-breakpoint
CREATE INDEX "idx_dispatch_jobs_read_application" ON "dispatch_jobs_read" ("application");--> statement-breakpoint
CREATE INDEX "idx_dispatch_jobs_read_subscription_id" ON "dispatch_jobs_read" ("subscription_id");--> statement-breakpoint
CREATE INDEX "idx_dispatch_jobs_read_message_group" ON "dispatch_jobs_read" ("message_group");--> statement-breakpoint
CREATE INDEX "idx_dispatch_jobs_read_created_at" ON "dispatch_jobs_read" ("created_at");--> statement-breakpoint
CREATE UNIQUE INDEX "idx_dispatch_pools_code_client" ON "dispatch_pools" ("code","client_id");--> statement-breakpoint
CREATE INDEX "idx_dispatch_pools_status" ON "dispatch_pools" ("status");--> statement-breakpoint
CREATE INDEX "idx_dispatch_pools_client_id" ON "dispatch_pools" ("client_id");--> statement-breakpoint
CREATE INDEX "idx_edm_additional_clients_mapping" ON "email_domain_mapping_additional_clients" ("email_domain_mapping_id");--> statement-breakpoint
CREATE INDEX "idx_edm_allowed_roles_mapping" ON "email_domain_mapping_allowed_roles" ("email_domain_mapping_id");--> statement-breakpoint
CREATE INDEX "idx_edm_granted_clients_mapping" ON "email_domain_mapping_granted_clients" ("email_domain_mapping_id");--> statement-breakpoint
CREATE UNIQUE INDEX "idx_email_domain_mappings_domain" ON "email_domain_mappings" ("email_domain");--> statement-breakpoint
CREATE INDEX "idx_email_domain_mappings_idp" ON "email_domain_mappings" ("identity_provider_id");--> statement-breakpoint
CREATE INDEX "idx_email_domain_mappings_scope" ON "email_domain_mappings" ("scope_type");--> statement-breakpoint
CREATE INDEX "idx_event_projection_feed_unprocessed" ON "event_projection_feed" ("id") WHERE "processed" = 0;--> statement-breakpoint
CREATE INDEX "idx_event_projection_feed_in_progress" ON "event_projection_feed" ("id") WHERE "processed" = 9;--> statement-breakpoint
CREATE INDEX "idx_spec_versions_event_type" ON "event_type_spec_versions" ("event_type_id");--> statement-breakpoint
CREATE INDEX "idx_spec_versions_status" ON "event_type_spec_versions" ("status");--> statement-breakpoint
CREATE INDEX "idx_event_types_code" ON "event_types" ("code");--> statement-breakpoint
CREATE INDEX "idx_event_types_status" ON "event_types" ("status");--> statement-breakpoint
CREATE INDEX "idx_event_types_source" ON "event_types" ("source");--> statement-breakpoint
CREATE INDEX "idx_event_types_application" ON "event_types" ("application");--> statement-breakpoint
CREATE INDEX "idx_event_types_subdomain" ON "event_types" ("subdomain");--> statement-breakpoint
CREATE INDEX "idx_event_types_aggregate" ON "event_types" ("aggregate");--> statement-breakpoint
CREATE INDEX "idx_events_type" ON "events" ("type");--> statement-breakpoint
CREATE INDEX "idx_events_client_type" ON "events" ("client_id","type");--> statement-breakpoint
CREATE INDEX "idx_events_time" ON "events" ("time");--> statement-breakpoint
CREATE INDEX "idx_events_correlation" ON "events" ("correlation_id");--> statement-breakpoint
CREATE UNIQUE INDEX "idx_events_deduplication" ON "events" ("deduplication_id");--> statement-breakpoint
CREATE INDEX "idx_events_read_type" ON "events_read" ("type");--> statement-breakpoint
CREATE INDEX "idx_events_read_client_id" ON "events_read" ("client_id");--> statement-breakpoint
CREATE INDEX "idx_events_read_time" ON "events_read" ("time");--> statement-breakpoint
CREATE INDEX "idx_events_read_application" ON "events_read" ("application");--> statement-breakpoint
CREATE INDEX "idx_events_read_subdomain" ON "events_read" ("subdomain");--> statement-breakpoint
CREATE INDEX "idx_events_read_aggregate" ON "events_read" ("aggregate");--> statement-breakpoint
CREATE INDEX "idx_events_read_correlation_id" ON "events_read" ("correlation_id");--> statement-breakpoint
CREATE INDEX "idx_idp_allowed_domains_idp" ON "identity_provider_allowed_domains" ("identity_provider_id");--> statement-breakpoint
CREATE UNIQUE INDEX "idx_identity_providers_code" ON "identity_providers" ("code");--> statement-breakpoint
CREATE UNIQUE INDEX "idx_idp_role_mappings_idp_role_name" ON "idp_role_mappings" ("idp_role_name");--> statement-breakpoint
CREATE INDEX "idx_oauth_client_allowed_origins_client" ON "oauth_client_allowed_origins" ("oauth_client_id");--> statement-breakpoint
CREATE INDEX "idx_oauth_client_allowed_origins_origin" ON "oauth_client_allowed_origins" ("allowed_origin");--> statement-breakpoint
CREATE INDEX "idx_oauth_client_application_ids_client" ON "oauth_client_application_ids" ("oauth_client_id");--> statement-breakpoint
CREATE INDEX "idx_oauth_client_grant_types_client" ON "oauth_client_grant_types" ("oauth_client_id");--> statement-breakpoint
CREATE INDEX "idx_oauth_client_redirect_uris_client" ON "oauth_client_redirect_uris" ("oauth_client_id");--> statement-breakpoint
CREATE INDEX "oauth_clients_client_id_idx" ON "oauth_clients" ("client_id");--> statement-breakpoint
CREATE INDEX "oauth_clients_active_idx" ON "oauth_clients" ("active");--> statement-breakpoint
CREATE INDEX "idx_oidc_login_states_expires" ON "oidc_login_states" ("expires_at");--> statement-breakpoint
CREATE INDEX "oidc_payloads_grant_id_idx" ON "oidc_payloads" ("grant_id");--> statement-breakpoint
CREATE INDEX "oidc_payloads_user_code_idx" ON "oidc_payloads" ("user_code");--> statement-breakpoint
CREATE INDEX "oidc_payloads_uid_idx" ON "oidc_payloads" ("uid");--> statement-breakpoint
CREATE INDEX "oidc_payloads_type_idx" ON "oidc_payloads" ("type");--> statement-breakpoint
CREATE INDEX "oidc_payloads_expires_at_idx" ON "oidc_payloads" ("expires_at");--> statement-breakpoint
CREATE INDEX "idx_config_access_app" ON "platform_config_access" ("application_code");--> statement-breakpoint
CREATE INDEX "idx_config_access_role" ON "platform_config_access" ("role_code");--> statement-breakpoint
CREATE INDEX "idx_platform_configs_lookup" ON "platform_configs" ("application_code","section","scope","client_id");--> statement-breakpoint
CREATE INDEX "idx_platform_configs_app_section" ON "platform_configs" ("application_code","section");--> statement-breakpoint
CREATE INDEX "idx_principal_app_access_app_id" ON "principal_application_access" ("application_id");--> statement-breakpoint
CREATE INDEX "idx_principal_roles_role_name" ON "principal_roles" ("role_name");--> statement-breakpoint
CREATE INDEX "idx_principal_roles_assigned_at" ON "principal_roles" ("assigned_at");--> statement-breakpoint
CREATE INDEX "idx_principals_type" ON "principals" ("type");--> statement-breakpoint
CREATE INDEX "idx_principals_client_id" ON "principals" ("client_id");--> statement-breakpoint
CREATE INDEX "idx_principals_active" ON "principals" ("active");--> statement-breakpoint
CREATE UNIQUE INDEX "idx_principals_email" ON "principals" ("email");--> statement-breakpoint
CREATE INDEX "idx_principals_email_domain" ON "principals" ("email_domain");--> statement-breakpoint
CREATE UNIQUE INDEX "idx_principals_service_account_id" ON "principals" ("service_account_id");--> statement-breakpoint
CREATE UNIQUE INDEX "idx_service_accounts_code" ON "service_accounts" ("code");--> statement-breakpoint
CREATE INDEX "idx_service_accounts_application_id" ON "service_accounts" ("application_id");--> statement-breakpoint
CREATE INDEX "idx_service_accounts_active" ON "service_accounts" ("active");--> statement-breakpoint
CREATE INDEX "idx_sub_configs_subscription" ON "subscription_custom_configs" ("subscription_id");--> statement-breakpoint
CREATE INDEX "idx_sub_event_types_subscription" ON "subscription_event_types" ("subscription_id");--> statement-breakpoint
CREATE INDEX "idx_sub_event_types_event_type" ON "subscription_event_types" ("event_type_id");--> statement-breakpoint
CREATE UNIQUE INDEX "idx_subscriptions_code_client" ON "subscriptions" ("code","client_id");--> statement-breakpoint
CREATE INDEX "idx_subscriptions_status" ON "subscriptions" ("status");--> statement-breakpoint
CREATE INDEX "idx_subscriptions_client_id" ON "subscriptions" ("client_id");--> statement-breakpoint
CREATE INDEX "idx_subscriptions_source" ON "subscriptions" ("source");--> statement-breakpoint
CREATE INDEX "idx_subscriptions_dispatch_pool" ON "subscriptions" ("dispatch_pool_id");--> statement-breakpoint
ALTER TABLE "oauth_client_allowed_origins" ADD CONSTRAINT "oauth_client_allowed_origins_v1HZdHOs0PKp_fkey" FOREIGN KEY ("oauth_client_id") REFERENCES "oauth_clients"("id") ON DELETE CASCADE;--> statement-breakpoint
ALTER TABLE "oauth_client_application_ids" ADD CONSTRAINT "oauth_client_application_ids_rkmPMUhQYrzk_fkey" FOREIGN KEY ("oauth_client_id") REFERENCES "oauth_clients"("id") ON DELETE CASCADE;--> statement-breakpoint
ALTER TABLE "oauth_client_grant_types" ADD CONSTRAINT "oauth_client_grant_types_oauth_client_id_oauth_clients_id_fkey" FOREIGN KEY ("oauth_client_id") REFERENCES "oauth_clients"("id") ON DELETE CASCADE;--> statement-breakpoint
ALTER TABLE "oauth_client_redirect_uris" ADD CONSTRAINT "oauth_client_redirect_uris_2MtuKyU2pJ9Z_fkey" FOREIGN KEY ("oauth_client_id") REFERENCES "oauth_clients"("id") ON DELETE CASCADE;--> statement-breakpoint
ALTER TABLE "principal_roles" ADD CONSTRAINT "principal_roles_principal_id_principals_id_fkey" FOREIGN KEY ("principal_id") REFERENCES "principals"("id") ON DELETE CASCADE;