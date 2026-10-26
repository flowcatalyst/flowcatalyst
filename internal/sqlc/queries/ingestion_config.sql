-- Reads the ingestion feature makes of platform config, section "ingest": the
-- per-environment settings a source names (docs/ingestion-design.md section
-- 13). The entries are written through the ingestion (SetIngestionConfig),
-- which persists them with the platform config repository; these only read.

-- One entry: the CLIENT-scoped value of the application at the client.
-- name: IngestionConfigEntryFind :one
SELECT value_type, value, updated_at FROM app_platform_configs
WHERE application_code = @application_code AND section = 'ingest' AND scope = 'CLIENT'
  AND client_id = @client_id AND property = @property;

-- Every entry, for a poll to resolve all the ingestions it lists with one read
-- (the table of entries is small: one per environment setting).
-- name: IngestionConfigEntriesList :many
SELECT application_code, client_id, property, value_type, value, updated_at
FROM app_platform_configs
WHERE section = 'ingest' AND scope = 'CLIENT' AND client_id IS NOT NULL;

-- name: IngestionApplicationCode :one
SELECT code FROM app_applications WHERE id = $1;

-- The ingestions of an application and client whose live or latest saved
-- version names the entry: by its key, or as "config: true" (the ingestion's
-- own code). jsonb_typeof keeps the string "true" from reading as the boolean.
-- name: IngestionsNamingConfig :many
SELECT DISTINCT i.id
FROM ing_ingestions i
JOIN ing_versions v ON v.ingestion_id = i.id AND (v.version = i.version OR v.version = i.latest_version)
WHERE i.application_id = @application_id AND i.client_id = @client_id
  AND (
    (jsonb_typeof(v.definition->'source'->'config') = 'string' AND v.definition->'source'->>'config' = @key::text)
    OR (v.definition->'source'->'config' = 'true'::jsonb AND i.code = @key::text)
  )
ORDER BY i.id;
