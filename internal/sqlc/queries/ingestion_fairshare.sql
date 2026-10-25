-- The runtimes that polled within the lease window, with what they carry: a
-- runtime's fair share of the polled ingestions of a kind is counted over the
-- live runtimes that carry that kind (the asking runtime has just been
-- upserted, so it counts itself).
-- name: IngestionRuntimesLiveKinds :many
SELECT runtime_id, kinds, builtin_kinds FROM ing_runtimes
WHERE last_seen_at > @since AND last_seen_at <= @until
ORDER BY runtime_id;

-- The leases a runtime holds and has not released, to be renewed first.
-- name: IngestionLeasesHeldBy :many
SELECT ingestion_id FROM ing_leases
WHERE runtime_id = @runtime_id AND NOT released AND expires_at > @now;

-- Serialises the lease section of concurrent polls: each poll renews what it
-- holds and then claims what is free, and two polls doing that in different
-- row orders would deadlock.
-- name: IngestionLeasesLock :exec
SELECT pg_advisory_xact_lock(hashtext('ingestion-leases'));
