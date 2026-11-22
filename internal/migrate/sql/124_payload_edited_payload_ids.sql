-- +goose Up
-- The audit event of a payload edit, platform:messaging:dispatch-job:payload-edited,
-- now records the payload rows the edit replaced and made
-- (docs/payload-offload-design.md, section 3.1): previousPayloadId, the
-- msg_payloads row the job pointed at before the edit, left out when its
-- payload was the legacy payload column; payloadId, the row the edit gave the
-- job, left out when FC_PAYLOAD_TABLE_WRITES is off (the edit then writes the
-- payload column). Both are optional, so the data of an edit made by an older
-- instance still validates, and the version stays 1.0. The seed now holds this
-- schema, but the seeder never replaces an attached 1.0, so this replaces it
-- for existing databases, only while it is still exactly what a seed wrote
-- (migration 072's rule): a schema someone changed is theirs and is left alone.
-- Idempotent.
--
-- Cost: one UPDATE of at most one row of msg_event_type_spec_versions, a small
-- configuration table (docs/large-table-schema-changes.md does not apply). No
-- schema change, so no effect probe (internal/migrate/probes.go: data-only).
UPDATE msg_event_type_spec_versions sv
   SET schema_content = '{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"contentType":{"type":"string"},"dispatchJobId":{"type":"string"},"payloadId":{"type":["string","null"]},"previousContentType":{"type":"string"},"previousPayloadId":{"type":["string","null"]},"previousSha256":{"type":"string"},"previousSize":{"minimum":0,"type":"integer"},"sha256":{"type":"string"},"size":{"minimum":0,"type":"integer"},"status":{"type":"string"}},"required":["dispatchJobId","status","previousSize","previousSha256","size","sha256","contentType","previousContentType"],"type":"object"}'::jsonb,
       updated_at = NOW()
  FROM msg_event_types et
 WHERE et.id = sv.event_type_id
   AND et.code = 'platform:messaging:dispatch-job:payload-edited'
   AND sv.version = '1.0'
   AND sv.schema_content = '{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"contentType":{"type":"string"},"dispatchJobId":{"type":"string"},"previousContentType":{"type":"string"},"previousSha256":{"type":"string"},"previousSize":{"minimum":0,"type":"integer"},"sha256":{"type":"string"},"size":{"minimum":0,"type":"integer"},"status":{"type":"string"}},"required":["dispatchJobId","status","previousSize","previousSha256","size","sha256","contentType","previousContentType"],"type":"object"}'::jsonb;

-- +goose Down
-- Puts back the schema a seed wrote before, only where the 1.0 is still this
-- one.
UPDATE msg_event_type_spec_versions sv
   SET schema_content = '{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"contentType":{"type":"string"},"dispatchJobId":{"type":"string"},"previousContentType":{"type":"string"},"previousSha256":{"type":"string"},"previousSize":{"minimum":0,"type":"integer"},"sha256":{"type":"string"},"size":{"minimum":0,"type":"integer"},"status":{"type":"string"}},"required":["dispatchJobId","status","previousSize","previousSha256","size","sha256","contentType","previousContentType"],"type":"object"}'::jsonb,
       updated_at = NOW()
  FROM msg_event_types et
 WHERE et.id = sv.event_type_id
   AND et.code = 'platform:messaging:dispatch-job:payload-edited'
   AND sv.version = '1.0'
   AND sv.schema_content = '{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"contentType":{"type":"string"},"dispatchJobId":{"type":"string"},"payloadId":{"type":["string","null"]},"previousContentType":{"type":"string"},"previousPayloadId":{"type":["string","null"]},"previousSha256":{"type":"string"},"previousSize":{"minimum":0,"type":"integer"},"sha256":{"type":"string"},"size":{"minimum":0,"type":"integer"},"status":{"type":"string"}},"required":["dispatchJobId","status","previousSize","previousSha256","size","sha256","contentType","previousContentType"],"type":"object"}'::jsonb;
