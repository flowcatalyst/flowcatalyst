-- +goose Up
-- Two platform events carry fields their seeded 1.0 schemas refuse
-- (additionalProperties is false): platform:iam:application:updated sends
-- type (the application's type after the update) and previousType (only when
-- the update changed it), and platform:ingestion:ingestion:config-set sends
-- secretsSet (the names of the secrets the same write set, null when none).
-- The owner's decision D7 (2026-11-22): the schemas admit them. All three are
-- optional, so the data of an event an older instance emitted still validates,
-- and the version stays 1.0. The seed now holds these schemas, but the seeder
-- never replaces an attached 1.0, so this replaces each for existing
-- databases, only while it is still exactly what a seed wrote (the rule of
-- migrations 072 and 124; application:updated's is the form 072 wrote, and a
-- database with the form before it gets 072 first): a schema someone changed
-- is theirs and is left alone. Idempotent.
--
-- Cost: an UPDATE of at most two rows of msg_event_type_spec_versions, a small
-- configuration table (docs/large-table-schema-changes.md does not apply). No
-- schema change, so no effect probe (internal/migrate/probes.go: data-only).
UPDATE msg_event_type_spec_versions sv
   SET schema_content = s.new_schema::jsonb, updated_at = NOW()
  FROM msg_event_types et,
       (VALUES
        ('platform:iam:application:updated',
         '{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"applicationId":{"type":"string"},"name":{"type":"string"}},"required":["applicationId","name"],"type":"object"}',
         '{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"applicationId":{"type":"string"},"name":{"type":"string"},"previousType":{"type":["string","null"]},"type":{"type":["string","null"]}},"required":["applicationId","name"],"type":"object"}'),
        ('platform:ingestion:ingestion:config-set',
         '{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"code":{"type":"string"},"ingestionId":{"type":"string"},"ingestions":{"items":{"type":"string"},"type":"array"},"key":{"type":"string"},"settings":{"items":{"type":"string"},"type":"array"}},"required":["ingestionId","code","key","settings","ingestions"],"type":"object"}',
         '{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"code":{"type":"string"},"ingestionId":{"type":"string"},"ingestions":{"items":{"type":"string"},"type":"array"},"key":{"type":"string"},"secretsSet":{"items":{"type":"string"},"type":["array","null"]},"settings":{"items":{"type":"string"},"type":"array"}},"required":["ingestionId","code","key","settings","ingestions"],"type":"object"}')
       ) AS s(code, old_schema, new_schema)
 WHERE et.id = sv.event_type_id
   AND et.code = s.code
   AND sv.version = '1.0'
   AND sv.schema_content = s.old_schema::jsonb;

-- +goose Down
-- Puts back the schemas a seed wrote before, only where the 1.0 is still the
-- one Up wrote.
UPDATE msg_event_type_spec_versions sv
   SET schema_content = s.old_schema::jsonb, updated_at = NOW()
  FROM msg_event_types et,
       (VALUES
        ('platform:iam:application:updated',
         '{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"applicationId":{"type":"string"},"name":{"type":"string"}},"required":["applicationId","name"],"type":"object"}',
         '{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"applicationId":{"type":"string"},"name":{"type":"string"},"previousType":{"type":["string","null"]},"type":{"type":["string","null"]}},"required":["applicationId","name"],"type":"object"}'),
        ('platform:ingestion:ingestion:config-set',
         '{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"code":{"type":"string"},"ingestionId":{"type":"string"},"ingestions":{"items":{"type":"string"},"type":"array"},"key":{"type":"string"},"settings":{"items":{"type":"string"},"type":"array"}},"required":["ingestionId","code","key","settings","ingestions"],"type":"object"}',
         '{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"code":{"type":"string"},"ingestionId":{"type":"string"},"ingestions":{"items":{"type":"string"},"type":"array"},"key":{"type":"string"},"secretsSet":{"items":{"type":"string"},"type":["array","null"]},"settings":{"items":{"type":"string"},"type":"array"}},"required":["ingestionId","code","key","settings","ingestions"],"type":"object"}')
       ) AS s(code, old_schema, new_schema)
 WHERE et.id = sv.event_type_id
   AND et.code = s.code
   AND sv.version = '1.0'
   AND sv.schema_content = s.new_schema::jsonb;
