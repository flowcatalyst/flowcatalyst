-- Hand the partitioned messaging tables over to pg_partman.
--
-- Production-only. fc-dev (embedded postgres) doesn't have the extension and
-- keeps the messaging tables as plain (non-partitioned) tables, so this
-- migration is registered alongside 019/022 in the production_migrations
-- list, not core_migrations.
--
-- Assumes pg_partman is already installed at the cluster level by IaC:
--   CREATE EXTENSION pg_partman SCHEMA partman;
--   GRANT ALL ON SCHEMA partman TO inhance_admin;
--
-- Behaviour:
--   1. Drops every existing child partition under each parent. Existing data
--      is discarded — this is a dev-environment-only takeover; production
--      runs of this migration should have no traffic on these tables yet.
--   2. Registers each parent with partman.create_parent (native, monthly,
--      premake = 3) if not already registered.
--   3. Sets retention = 90 days, retention_keep_table = false on each set.
--   4. Calls partman.run_maintenance() once to materialize the initial
--      partition ring before bgw takes over.
--
-- After this migration, the in-Rust PartitionManagerService is removed;
-- pg_partman_bgw handles forward and retention maintenance from here on.
--
-- Version handling: pg_partman 5.x removed the `p_type` argument that
-- pg_partman 4.x required (everything is native partitioning now). The
-- migration detects the major version and calls the right signature.

DO $partman_takeover$
DECLARE
    v_version  TEXT;
    v_major    INT;
    parents    TEXT[] := ARRAY[
        'msg_events',
        'msg_events_read',
        'msg_dispatch_jobs',
        'msg_dispatch_jobs_read',
        'msg_dispatch_job_attempts',
        'msg_scheduled_job_instances',
        'msg_scheduled_job_instance_logs'
    ];
    parent     TEXT;
    child      RECORD;
    qualified  TEXT;
BEGIN
    SELECT extversion INTO v_version
    FROM pg_extension WHERE extname = 'pg_partman';

    IF v_version IS NULL THEN
        RAISE EXCEPTION
            'pg_partman extension is not installed. Run via IaC: CREATE EXTENSION pg_partman SCHEMA partman;';
    END IF;

    v_major := (string_to_array(v_version, '.'))[1]::INT;
    RAISE NOTICE 'pg_partman version: % (major: %)', v_version, v_major;

    FOREACH parent IN ARRAY parents LOOP
        qualified := 'public.' || parent;

        -- 1. Drop existing child partitions. CASCADE handles dependent indexes.
        FOR child IN
            SELECT c.relname AS name
            FROM pg_inherits i
            JOIN pg_class    p ON i.inhparent = p.oid
            JOIN pg_class    c ON i.inhrelid  = c.oid
            JOIN pg_namespace n ON n.oid      = p.relnamespace
            WHERE p.relname = parent AND n.nspname = 'public'
        LOOP
            EXECUTE format('DROP TABLE IF EXISTS public.%I CASCADE', child.name);
        END LOOP;

        -- 2. Register with partman if not already.
        IF NOT EXISTS (
            SELECT 1 FROM partman.part_config WHERE parent_table = qualified
        ) THEN
            IF v_major >= 5 THEN
                PERFORM partman.create_parent(
                    p_parent_table := qualified,
                    p_control      := 'created_at',
                    p_interval     := '1 month',
                    p_premake      := 3
                );
            ELSE
                PERFORM partman.create_parent(
                    p_parent_table := qualified,
                    p_control      := 'created_at',
                    p_type         := 'native',
                    p_interval     := '1 month',
                    p_premake      := 3
                );
            END IF;
            RAISE NOTICE 'partman.create_parent registered: %', qualified;
        ELSE
            RAISE NOTICE 'partman: % already registered, skipping', qualified;
        END IF;

        -- 3. Apply retention policy.
        UPDATE partman.part_config
        SET retention                = '90 days',
            retention_keep_table     = false,
            infinite_time_partitions = false
        WHERE parent_table = qualified;
    END LOOP;

    -- 4. Materialize forward partitions immediately so bgw doesn't have to
    --    wait until its first tick.
    PERFORM partman.run_maintenance();
END
$partman_takeover$;
