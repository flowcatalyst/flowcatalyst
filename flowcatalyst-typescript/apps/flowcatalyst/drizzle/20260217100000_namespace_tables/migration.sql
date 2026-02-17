-- Namespace database tables with domain prefixes
-- Prefixes: iam_ (Identity), oauth_ (OAuth/OIDC), tnt_ (Tenancy), msg_ (Messaging), aud_ (Audit), app_ (Applications)

-- ============================================================
-- IAM (Identity & Access Management)
-- ============================================================

ALTER TABLE "principals" RENAME TO "iam_principals";
ALTER TABLE "principal_roles" RENAME TO "iam_principal_roles";
ALTER TABLE "principal_application_access" RENAME TO "iam_principal_application_access";
ALTER TABLE "service_accounts" RENAME TO "iam_service_accounts";
ALTER TABLE "client_access_grants" RENAME TO "iam_client_access_grants";
ALTER TABLE "auth_roles" RENAME TO "iam_roles";
ALTER TABLE "auth_permissions" RENAME TO "iam_permissions";
ALTER TABLE "role_permissions" RENAME TO "iam_role_permissions";
--> statement-breakpoint

-- IAM indexes
ALTER INDEX "idx_principals_type" RENAME TO "idx_iam_principals_type";
ALTER INDEX "idx_principals_client_id" RENAME TO "idx_iam_principals_client_id";
ALTER INDEX "idx_principals_active" RENAME TO "idx_iam_principals_active";
ALTER INDEX "idx_principals_email" RENAME TO "idx_iam_principals_email";
ALTER INDEX "idx_principals_email_domain" RENAME TO "idx_iam_principals_email_domain";
ALTER INDEX "idx_principals_service_account_id" RENAME TO "idx_iam_principals_service_account_id";
ALTER INDEX "idx_principal_roles_role_name" RENAME TO "idx_iam_principal_roles_role_name";
ALTER INDEX "idx_principal_roles_assigned_at" RENAME TO "idx_iam_principal_roles_assigned_at";
ALTER INDEX "idx_principal_app_access_app_id" RENAME TO "idx_iam_principal_app_access_app_id";
ALTER INDEX "idx_service_accounts_code" RENAME TO "idx_iam_service_accounts_code";
ALTER INDEX "idx_service_accounts_application_id" RENAME TO "idx_iam_service_accounts_application_id";
ALTER INDEX "idx_service_accounts_active" RENAME TO "idx_iam_service_accounts_active";
ALTER INDEX "idx_client_access_grants_principal" RENAME TO "idx_iam_client_access_grants_principal";
ALTER INDEX "idx_client_access_grants_client" RENAME TO "idx_iam_client_access_grants_client";
ALTER INDEX "idx_auth_roles_name" RENAME TO "idx_iam_roles_name";
ALTER INDEX "idx_auth_roles_application_id" RENAME TO "idx_iam_roles_application_id";
ALTER INDEX "idx_auth_roles_application_code" RENAME TO "idx_iam_roles_application_code";
ALTER INDEX "idx_auth_roles_source" RENAME TO "idx_iam_roles_source";
ALTER INDEX "idx_auth_roles_client_managed" RENAME TO "idx_iam_roles_client_managed";
ALTER INDEX "idx_auth_permissions_code" RENAME TO "idx_iam_permissions_code";
ALTER INDEX "idx_auth_permissions_subdomain" RENAME TO "idx_iam_permissions_subdomain";
ALTER INDEX "idx_auth_permissions_context" RENAME TO "idx_iam_permissions_context";
ALTER INDEX "idx_role_permissions_role_id" RENAME TO "idx_iam_role_permissions_role_id";
--> statement-breakpoint

-- IAM unique constraints (Postgres treats unique constraints as indexes)
ALTER INDEX "uq_client_access_grants_principal_client" RENAME TO "uq_iam_client_access_grants_principal_client";
--> statement-breakpoint

-- ============================================================
-- OAuth / OIDC
-- ============================================================

-- oauth_clients, oauth_client_redirect_uris, oauth_client_allowed_origins,
-- oauth_client_grant_types, oauth_client_application_ids: already prefixed, no rename needed.

ALTER TABLE "oidc_payloads" RENAME TO "oauth_oidc_payloads";
ALTER TABLE "oidc_login_states" RENAME TO "oauth_oidc_login_states";
ALTER TABLE "identity_providers" RENAME TO "oauth_identity_providers";
ALTER TABLE "identity_provider_allowed_domains" RENAME TO "oauth_identity_provider_allowed_domains";
ALTER TABLE "idp_role_mappings" RENAME TO "oauth_idp_role_mappings";
--> statement-breakpoint

-- OAuth indexes
ALTER INDEX "oidc_payloads_grant_id_idx" RENAME TO "oauth_oidc_payloads_grant_id_idx";
ALTER INDEX "oidc_payloads_user_code_idx" RENAME TO "oauth_oidc_payloads_user_code_idx";
ALTER INDEX "oidc_payloads_uid_idx" RENAME TO "oauth_oidc_payloads_uid_idx";
ALTER INDEX "oidc_payloads_type_idx" RENAME TO "oauth_oidc_payloads_type_idx";
ALTER INDEX "oidc_payloads_expires_at_idx" RENAME TO "oauth_oidc_payloads_expires_at_idx";
ALTER INDEX "idx_oidc_login_states_expires" RENAME TO "idx_oauth_oidc_login_states_expires";
ALTER INDEX "idx_identity_providers_code" RENAME TO "idx_oauth_identity_providers_code";
ALTER INDEX "idx_idp_allowed_domains_idp" RENAME TO "idx_oauth_idp_allowed_domains_idp";
ALTER INDEX "idx_idp_role_mappings_idp_role_name" RENAME TO "idx_oauth_idp_role_mappings_idp_role_name";
--> statement-breakpoint

-- ============================================================
-- Tenancy
-- ============================================================

ALTER TABLE "clients" RENAME TO "tnt_clients";
ALTER TABLE "client_auth_configs" RENAME TO "tnt_client_auth_configs";
ALTER TABLE "anchor_domains" RENAME TO "tnt_anchor_domains";
ALTER TABLE "email_domain_mappings" RENAME TO "tnt_email_domain_mappings";
ALTER TABLE "email_domain_mapping_additional_clients" RENAME TO "tnt_email_domain_mapping_additional_clients";
ALTER TABLE "email_domain_mapping_granted_clients" RENAME TO "tnt_email_domain_mapping_granted_clients";
ALTER TABLE "email_domain_mapping_allowed_roles" RENAME TO "tnt_email_domain_mapping_allowed_roles";
ALTER TABLE "cors_allowed_origins" RENAME TO "tnt_cors_allowed_origins";
--> statement-breakpoint

-- Tenancy indexes
ALTER INDEX "idx_clients_identifier" RENAME TO "idx_tnt_clients_identifier";
ALTER INDEX "idx_clients_status" RENAME TO "idx_tnt_clients_status";
ALTER INDEX "client_auth_configs_email_domain_idx" RENAME TO "tnt_client_auth_configs_email_domain_idx";
ALTER INDEX "client_auth_configs_config_type_idx" RENAME TO "tnt_client_auth_configs_config_type_idx";
ALTER INDEX "client_auth_configs_primary_client_id_idx" RENAME TO "tnt_client_auth_configs_primary_client_id_idx";
ALTER INDEX "anchor_domains_domain_idx" RENAME TO "tnt_anchor_domains_domain_idx";
ALTER INDEX "idx_email_domain_mappings_domain" RENAME TO "idx_tnt_email_domain_mappings_domain";
ALTER INDEX "idx_email_domain_mappings_idp" RENAME TO "idx_tnt_email_domain_mappings_idp";
ALTER INDEX "idx_email_domain_mappings_scope" RENAME TO "idx_tnt_email_domain_mappings_scope";
ALTER INDEX "idx_edm_additional_clients_mapping" RENAME TO "idx_tnt_edm_additional_clients_mapping";
ALTER INDEX "idx_edm_granted_clients_mapping" RENAME TO "idx_tnt_edm_granted_clients_mapping";
ALTER INDEX "idx_edm_allowed_roles_mapping" RENAME TO "idx_tnt_edm_allowed_roles_mapping";
ALTER INDEX "cors_allowed_origins_origin_idx" RENAME TO "tnt_cors_allowed_origins_origin_idx";
--> statement-breakpoint

-- ============================================================
-- Messaging
-- ============================================================

ALTER TABLE "events" RENAME TO "msg_events";
ALTER TABLE "events_read" RENAME TO "msg_events_read";
ALTER TABLE "event_types" RENAME TO "msg_event_types";
ALTER TABLE "event_type_spec_versions" RENAME TO "msg_event_type_spec_versions";
ALTER TABLE "subscriptions" RENAME TO "msg_subscriptions";
ALTER TABLE "subscription_event_types" RENAME TO "msg_subscription_event_types";
ALTER TABLE "subscription_custom_configs" RENAME TO "msg_subscription_custom_configs";
ALTER TABLE "dispatch_jobs" RENAME TO "msg_dispatch_jobs";
ALTER TABLE "dispatch_jobs_read" RENAME TO "msg_dispatch_jobs_read";
ALTER TABLE "dispatch_job_attempts" RENAME TO "msg_dispatch_job_attempts";
ALTER TABLE "dispatch_pools" RENAME TO "msg_dispatch_pools";
ALTER TABLE "event_projection_feed" RENAME TO "msg_event_projection_feed";
ALTER TABLE "dispatch_job_projection_feed" RENAME TO "msg_dispatch_job_projection_feed";
--> statement-breakpoint

-- Messaging indexes
ALTER INDEX "idx_events_type" RENAME TO "idx_msg_events_type";
ALTER INDEX "idx_events_client_type" RENAME TO "idx_msg_events_client_type";
ALTER INDEX "idx_events_time" RENAME TO "idx_msg_events_time";
ALTER INDEX "idx_events_correlation" RENAME TO "idx_msg_events_correlation";
ALTER INDEX "idx_events_deduplication" RENAME TO "idx_msg_events_deduplication";
ALTER INDEX "idx_events_read_type" RENAME TO "idx_msg_events_read_type";
ALTER INDEX "idx_events_read_client_id" RENAME TO "idx_msg_events_read_client_id";
ALTER INDEX "idx_events_read_time" RENAME TO "idx_msg_events_read_time";
ALTER INDEX "idx_events_read_application" RENAME TO "idx_msg_events_read_application";
ALTER INDEX "idx_events_read_subdomain" RENAME TO "idx_msg_events_read_subdomain";
ALTER INDEX "idx_events_read_aggregate" RENAME TO "idx_msg_events_read_aggregate";
ALTER INDEX "idx_events_read_correlation_id" RENAME TO "idx_msg_events_read_correlation_id";
ALTER INDEX "idx_event_types_code" RENAME TO "idx_msg_event_types_code";
ALTER INDEX "idx_event_types_status" RENAME TO "idx_msg_event_types_status";
ALTER INDEX "idx_event_types_source" RENAME TO "idx_msg_event_types_source";
ALTER INDEX "idx_event_types_application" RENAME TO "idx_msg_event_types_application";
ALTER INDEX "idx_event_types_subdomain" RENAME TO "idx_msg_event_types_subdomain";
ALTER INDEX "idx_event_types_aggregate" RENAME TO "idx_msg_event_types_aggregate";
ALTER INDEX "idx_spec_versions_event_type" RENAME TO "idx_msg_spec_versions_event_type";
ALTER INDEX "idx_spec_versions_status" RENAME TO "idx_msg_spec_versions_status";
ALTER INDEX "idx_dispatch_jobs_status" RENAME TO "idx_msg_dispatch_jobs_status";
ALTER INDEX "idx_dispatch_jobs_client_id" RENAME TO "idx_msg_dispatch_jobs_client_id";
ALTER INDEX "idx_dispatch_jobs_message_group" RENAME TO "idx_msg_dispatch_jobs_message_group";
ALTER INDEX "idx_dispatch_jobs_subscription_id" RENAME TO "idx_msg_dispatch_jobs_subscription_id";
ALTER INDEX "idx_dispatch_jobs_created_at" RENAME TO "idx_msg_dispatch_jobs_created_at";
ALTER INDEX "idx_dispatch_jobs_scheduled_for" RENAME TO "idx_msg_dispatch_jobs_scheduled_for";
ALTER INDEX "idx_dispatch_jobs_read_status" RENAME TO "idx_msg_dispatch_jobs_read_status";
ALTER INDEX "idx_dispatch_jobs_read_client_id" RENAME TO "idx_msg_dispatch_jobs_read_client_id";
ALTER INDEX "idx_dispatch_jobs_read_application" RENAME TO "idx_msg_dispatch_jobs_read_application";
ALTER INDEX "idx_dispatch_jobs_read_subscription_id" RENAME TO "idx_msg_dispatch_jobs_read_subscription_id";
ALTER INDEX "idx_dispatch_jobs_read_message_group" RENAME TO "idx_msg_dispatch_jobs_read_message_group";
ALTER INDEX "idx_dispatch_jobs_read_created_at" RENAME TO "idx_msg_dispatch_jobs_read_created_at";
ALTER INDEX "idx_dispatch_job_attempts_job_number" RENAME TO "idx_msg_dispatch_job_attempts_job_number";
ALTER INDEX "idx_dispatch_job_attempts_job" RENAME TO "idx_msg_dispatch_job_attempts_job";
ALTER INDEX "idx_dispatch_pools_code_client" RENAME TO "idx_msg_dispatch_pools_code_client";
ALTER INDEX "idx_dispatch_pools_status" RENAME TO "idx_msg_dispatch_pools_status";
ALTER INDEX "idx_dispatch_pools_client_id" RENAME TO "idx_msg_dispatch_pools_client_id";
ALTER INDEX "idx_subscriptions_code_client" RENAME TO "idx_msg_subscriptions_code_client";
ALTER INDEX "idx_subscriptions_status" RENAME TO "idx_msg_subscriptions_status";
ALTER INDEX "idx_subscriptions_client_id" RENAME TO "idx_msg_subscriptions_client_id";
ALTER INDEX "idx_subscriptions_source" RENAME TO "idx_msg_subscriptions_source";
ALTER INDEX "idx_subscriptions_dispatch_pool" RENAME TO "idx_msg_subscriptions_dispatch_pool";
ALTER INDEX "idx_sub_event_types_subscription" RENAME TO "idx_msg_sub_event_types_subscription";
ALTER INDEX "idx_sub_event_types_event_type" RENAME TO "idx_msg_sub_event_types_event_type";
ALTER INDEX "idx_sub_configs_subscription" RENAME TO "idx_msg_sub_configs_subscription";
ALTER INDEX "idx_event_projection_feed_unprocessed" RENAME TO "idx_msg_event_projection_feed_unprocessed";
ALTER INDEX "idx_event_projection_feed_in_progress" RENAME TO "idx_msg_event_projection_feed_in_progress";
ALTER INDEX "idx_dj_projection_feed_unprocessed" RENAME TO "idx_msg_dj_projection_feed_unprocessed";
ALTER INDEX "idx_dj_projection_feed_in_progress" RENAME TO "idx_msg_dj_projection_feed_in_progress";
ALTER INDEX "idx_dj_projection_feed_processed_at" RENAME TO "idx_msg_dj_projection_feed_processed_at";
--> statement-breakpoint

-- Messaging unique constraints
ALTER INDEX "uq_spec_versions_event_type_version" RENAME TO "uq_msg_spec_versions_event_type_version";
--> statement-breakpoint

-- ============================================================
-- Audit
-- ============================================================

ALTER TABLE "audit_logs" RENAME TO "aud_logs";
--> statement-breakpoint

ALTER INDEX "idx_audit_logs_entity" RENAME TO "idx_aud_logs_entity";
ALTER INDEX "idx_audit_logs_performed" RENAME TO "idx_aud_logs_performed";
ALTER INDEX "idx_audit_logs_principal" RENAME TO "idx_aud_logs_principal";
ALTER INDEX "idx_audit_logs_operation" RENAME TO "idx_aud_logs_operation";
--> statement-breakpoint

-- ============================================================
-- Applications & Platform Config
-- ============================================================

ALTER TABLE "applications" RENAME TO "app_applications";
ALTER TABLE "application_client_configs" RENAME TO "app_client_configs";
ALTER TABLE "platform_configs" RENAME TO "app_platform_configs";
ALTER TABLE "platform_config_access" RENAME TO "app_platform_config_access";
--> statement-breakpoint

ALTER INDEX "idx_applications_code" RENAME TO "idx_app_applications_code";
ALTER INDEX "idx_applications_type" RENAME TO "idx_app_applications_type";
ALTER INDEX "idx_applications_active" RENAME TO "idx_app_applications_active";
ALTER INDEX "idx_app_client_configs_application" RENAME TO "idx_app_client_configs_app";
ALTER INDEX "idx_app_client_configs_client" RENAME TO "idx_app_client_configs_clt";
ALTER INDEX "idx_platform_configs_lookup" RENAME TO "idx_app_platform_configs_lookup";
ALTER INDEX "idx_platform_configs_app_section" RENAME TO "idx_app_platform_configs_app_section";
ALTER INDEX "idx_config_access_app" RENAME TO "idx_app_config_access_app";
ALTER INDEX "idx_config_access_role" RENAME TO "idx_app_config_access_role";
--> statement-breakpoint

-- App unique constraints
ALTER INDEX "uq_app_client_configs_app_client" RENAME TO "uq_app_client_configs_app_clt";
ALTER INDEX "uq_platform_config_key" RENAME TO "uq_app_platform_config_key";
ALTER INDEX "uq_config_access_role" RENAME TO "uq_app_config_access_role";
