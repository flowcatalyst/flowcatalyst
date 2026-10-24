-- The runtimes that polled within the lease window: the denominator of a
-- runtime's fair share of the polled ingestions (the asking runtime has just
-- been upserted, so it counts itself).
-- name: IngestionRuntimesLive :one
SELECT count(*)::bigint FROM ing_runtimes WHERE last_seen_at > @since AND last_seen_at <= @until;

-- The leases a runtime holds and has not released, to be renewed first.
-- name: IngestionLeasesHeldBy :many
SELECT ingestion_id FROM ing_leases
WHERE runtime_id = @runtime_id AND NOT released AND expires_at > @now;
