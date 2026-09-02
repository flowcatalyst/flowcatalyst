-- +goose Up
-- X-06 phase 3: CHECK constraints backing the strict (T, bool) enum parsers
-- landed in phase 2. Each constraint matches its Go const block exactly (see
-- the referenced entity.go for the source of truth) and enforces at the write
-- boundary what the read boundary already refuses to coerce: an unrecognised
-- value is a loud error, not a silent default.
--
-- Pre-scan: every column below was queried against a freshly-migrated test
-- database (schema + all migration-seeded bootstrap rows) for values outside
-- its allowed set. Zero violations found — see the X-06 phase 2/3 report for
-- the query. This does not scan a live production database; if a deployed
-- environment has accumulated a legacy value outside the allowed set, this
-- migration will fail loudly on that row rather than silently drop or coerce
-- it — which is the intended behaviour, but means the operator must resolve
-- the row before this migration can apply there.
--
-- Nullable columns (iam_service_accounts.wh_auth_type, iam_principals.scope)
-- get an `col IS NULL OR col IN (...)` form so NULL keeps passing.
--
-- msg_scheduled_job_instances and iam_login_attempts are RANGE-partitioned
-- parent tables (migrations 022, 049); ALTER TABLE ... ADD CONSTRAINT on a
-- partitioned parent validates and applies the constraint across every
-- existing partition (including the DEFAULT partition) and is inherited by
-- every future partition the housekeeping loop creates — no per-partition
-- migration needed.
--
-- Each ADD CONSTRAINT is guarded by a pg_constraint existence check
-- (PostgreSQL has no `ADD CONSTRAINT IF NOT EXISTS`), matching the
-- idempotency discipline the rest of this directory uses.

-- serviceaccount.WebhookAuthType — internal/platform/serviceaccount/entity.go
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_iam_service_accounts_wh_auth_type'
    ) THEN
        ALTER TABLE iam_service_accounts
            ADD CONSTRAINT chk_iam_service_accounts_wh_auth_type
            CHECK (wh_auth_type IS NULL OR wh_auth_type IN ('NONE', 'BEARER_TOKEN', 'BASIC_AUTH', 'API_KEY', 'HMAC_SIGNATURE'));
    END IF;
END $$;
-- +goose StatementEnd

-- loginattempt.Outcome — internal/platform/loginattempt/loginattempt.go (partitioned parent; propagates to all partitions)
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_iam_login_attempts_outcome'
    ) THEN
        ALTER TABLE iam_login_attempts
            ADD CONSTRAINT chk_iam_login_attempts_outcome
            CHECK (outcome IN ('SUCCESS', 'FAILURE'));
    END IF;
END $$;
-- +goose StatementEnd

-- passwordreset.Purpose — internal/platform/passwordreset/passwordreset.go
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_iam_password_reset_tokens_purpose'
    ) THEN
        ALTER TABLE iam_password_reset_tokens
            ADD CONSTRAINT chk_iam_password_reset_tokens_purpose
            CHECK (purpose IN ('reset', 'invite'));
    END IF;
END $$;
-- +goose StatementEnd

-- dispatchpool.Status — internal/platform/dispatchpool/entity.go
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_msg_dispatch_pools_status'
    ) THEN
        ALTER TABLE msg_dispatch_pools
            ADD CONSTRAINT chk_msg_dispatch_pools_status
            CHECK (status IN ('ACTIVE', 'SUSPENDED', 'ARCHIVED'));
    END IF;
END $$;
-- +goose StatementEnd

-- client.Status — internal/platform/client/entity.go
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_tnt_clients_status'
    ) THEN
        ALTER TABLE tnt_clients
            ADD CONSTRAINT chk_tnt_clients_status
            CHECK (status IN ('ACTIVE', 'INACTIVE', 'SUSPENDED'));
    END IF;
END $$;
-- +goose StatementEnd

-- process.Status — internal/platform/process/entity.go
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_msg_processes_status'
    ) THEN
        ALTER TABLE msg_processes
            ADD CONSTRAINT chk_msg_processes_status
            CHECK (status IN ('CURRENT', 'ARCHIVED'));
    END IF;
END $$;
-- +goose StatementEnd

-- process.Source — internal/platform/process/entity.go
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_msg_processes_source'
    ) THEN
        ALTER TABLE msg_processes
            ADD CONSTRAINT chk_msg_processes_source
            CHECK (source IN ('CODE', 'API', 'UI'));
    END IF;
END $$;
-- +goose StatementEnd

-- application.Type — internal/platform/application/entity.go
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_app_applications_type'
    ) THEN
        ALTER TABLE app_applications
            ADD CONSTRAINT chk_app_applications_type
            CHECK (type IN ('APPLICATION', 'INTEGRATION'));
    END IF;
END $$;
-- +goose StatementEnd

-- subscription.Status — internal/platform/subscription/entity.go
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_msg_subscriptions_status'
    ) THEN
        ALTER TABLE msg_subscriptions
            ADD CONSTRAINT chk_msg_subscriptions_status
            CHECK (status IN ('ACTIVE', 'PAUSED'));
    END IF;
END $$;
-- +goose StatementEnd

-- subscription.Source — internal/platform/subscription/entity.go
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_msg_subscriptions_source'
    ) THEN
        ALTER TABLE msg_subscriptions
            ADD CONSTRAINT chk_msg_subscriptions_source
            CHECK (source IN ('CODE', 'API', 'UI'));
    END IF;
END $$;
-- +goose StatementEnd

-- eventtype.Status — internal/platform/eventtype/entity.go
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_msg_event_types_status'
    ) THEN
        ALTER TABLE msg_event_types
            ADD CONSTRAINT chk_msg_event_types_status
            CHECK (status IN ('CURRENT', 'ARCHIVED'));
    END IF;
END $$;
-- +goose StatementEnd

-- eventtype.Source — internal/platform/eventtype/entity.go
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_msg_event_types_source'
    ) THEN
        ALTER TABLE msg_event_types
            ADD CONSTRAINT chk_msg_event_types_source
            CHECK (source IN ('CODE', 'API', 'UI'));
    END IF;
END $$;
-- +goose StatementEnd

-- eventtype.SchemaType — internal/platform/eventtype/entity.go (XML_SCHEMA/PROTOBUF are accepted legacy aliases of XSD/PROTO, not just wire input)
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_msg_event_type_spec_versions_schema_type'
    ) THEN
        ALTER TABLE msg_event_type_spec_versions
            ADD CONSTRAINT chk_msg_event_type_spec_versions_schema_type
            CHECK (schema_type IN ('JSON_SCHEMA', 'XSD', 'XML_SCHEMA', 'PROTO', 'PROTOBUF'));
    END IF;
END $$;
-- +goose StatementEnd

-- eventtype.SpecVersionStatus — internal/platform/eventtype/entity.go
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_msg_event_type_spec_versions_status'
    ) THEN
        ALTER TABLE msg_event_type_spec_versions
            ADD CONSTRAINT chk_msg_event_type_spec_versions_status
            CHECK (status IN ('FINALISING', 'CURRENT', 'DEPRECATED'));
    END IF;
END $$;
-- +goose StatementEnd

-- principal.Type — internal/platform/principal/entity.go
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_iam_principals_type'
    ) THEN
        ALTER TABLE iam_principals
            ADD CONSTRAINT chk_iam_principals_type
            CHECK (type IN ('USER', 'SERVICE'));
    END IF;
END $$;
-- +goose StatementEnd

-- principal.UserScope — internal/platform/principal/entity.go
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_iam_principals_scope'
    ) THEN
        ALTER TABLE iam_principals
            ADD CONSTRAINT chk_iam_principals_scope
            CHECK (scope IS NULL OR scope IN ('ANCHOR', 'PARTNER', 'CLIENT'));
    END IF;
END $$;
-- +goose StatementEnd

-- role.Source — internal/platform/role/entity.go
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_iam_roles_source'
    ) THEN
        ALTER TABLE iam_roles
            ADD CONSTRAINT chk_iam_roles_source
            CHECK (source IN ('CODE', 'DATABASE', 'SDK'));
    END IF;
END $$;
-- +goose StatementEnd

-- connection.Status — internal/platform/connection/entity.go
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_msg_connections_status'
    ) THEN
        ALTER TABLE msg_connections
            ADD CONSTRAINT chk_msg_connections_status
            CHECK (status IN ('ACTIVE', 'PAUSED'));
    END IF;
END $$;
-- +goose StatementEnd

-- auth.OAuthClientType — internal/platform/auth/entity.go
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_oauth_clients_client_type'
    ) THEN
        ALTER TABLE oauth_clients
            ADD CONSTRAINT chk_oauth_clients_client_type
            CHECK (client_type IN ('PUBLIC', 'CONFIDENTIAL'));
    END IF;
END $$;
-- +goose StatementEnd

-- auth.AuthConfigType — internal/platform/auth/entity.go
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_tnt_client_auth_configs_config_type'
    ) THEN
        ALTER TABLE tnt_client_auth_configs
            ADD CONSTRAINT chk_tnt_client_auth_configs_config_type
            CHECK (config_type IN ('ANCHOR', 'PARTNER', 'CLIENT'));
    END IF;
END $$;
-- +goose StatementEnd

-- auth.AuthProvider — internal/platform/auth/entity.go
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_tnt_client_auth_configs_auth_provider'
    ) THEN
        ALTER TABLE tnt_client_auth_configs
            ADD CONSTRAINT chk_tnt_client_auth_configs_auth_provider
            CHECK (auth_provider IN ('INTERNAL', 'OIDC'));
    END IF;
END $$;
-- +goose StatementEnd

-- identityprovider.Type — internal/platform/identityprovider/entity.go
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_oauth_identity_providers_type'
    ) THEN
        ALTER TABLE oauth_identity_providers
            ADD CONSTRAINT chk_oauth_identity_providers_type
            CHECK (type IN ('INTERNAL', 'OIDC'));
    END IF;
END $$;
-- +goose StatementEnd

-- emaildomainmapping.ScopeType — internal/platform/emaildomainmapping/entity.go
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_tnt_email_domain_mappings_scope_type'
    ) THEN
        ALTER TABLE tnt_email_domain_mappings
            ADD CONSTRAINT chk_tnt_email_domain_mappings_scope_type
            CHECK (scope_type IN ('ANCHOR', 'PARTNER', 'CLIENT'));
    END IF;
END $$;
-- +goose StatementEnd

-- platformconfig.Scope — internal/platform/platformconfig/entity.go
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_app_platform_configs_scope'
    ) THEN
        ALTER TABLE app_platform_configs
            ADD CONSTRAINT chk_app_platform_configs_scope
            CHECK (scope IN ('GLOBAL', 'CLIENT'));
    END IF;
END $$;
-- +goose StatementEnd

-- platformconfig.ValueType — internal/platform/platformconfig/entity.go
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_app_platform_configs_value_type'
    ) THEN
        ALTER TABLE app_platform_configs
            ADD CONSTRAINT chk_app_platform_configs_value_type
            CHECK (value_type IN ('PLAIN', 'SECRET'));
    END IF;
END $$;
-- +goose StatementEnd

-- openapispecs.Status — internal/platform/openapispecs/entity.go
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_app_application_openapi_specs_status'
    ) THEN
        ALTER TABLE app_application_openapi_specs
            ADD CONSTRAINT chk_app_application_openapi_specs_status
            CHECK (status IN ('CURRENT', 'ARCHIVED'));
    END IF;
END $$;
-- +goose StatementEnd

-- scheduledjob.Status — internal/platform/scheduledjob/entity.go
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_msg_scheduled_jobs_status'
    ) THEN
        ALTER TABLE msg_scheduled_jobs
            ADD CONSTRAINT chk_msg_scheduled_jobs_status
            CHECK (status IN ('ACTIVE', 'PAUSED', 'ARCHIVED'));
    END IF;
END $$;
-- +goose StatementEnd

-- scheduledjob.InstanceStatus — internal/platform/scheduledjob/instance.go (partitioned parent; propagates to all partitions)
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_msg_scheduled_job_instances_status'
    ) THEN
        ALTER TABLE msg_scheduled_job_instances
            ADD CONSTRAINT chk_msg_scheduled_job_instances_status
            CHECK (status IN ('QUEUED', 'IN_FLIGHT', 'DELIVERED', 'COMPLETED', 'FAILED', 'DELIVERY_FAILED'));
    END IF;
END $$;
-- +goose StatementEnd

-- scheduledjob.TriggerKind — internal/platform/scheduledjob/instance.go (partitioned parent; propagates to all partitions)
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_msg_scheduled_job_instances_trigger_kind'
    ) THEN
        ALTER TABLE msg_scheduled_job_instances
            ADD CONSTRAINT chk_msg_scheduled_job_instances_trigger_kind
            CHECK (trigger_kind IN ('CRON', 'MANUAL', 'BACKFILL'));
    END IF;
END $$;
-- +goose StatementEnd

