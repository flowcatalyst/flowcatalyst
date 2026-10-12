-- +goose Up
-- The function events belong in the platform:function:* family Java and Rust
-- emit (owner ruling 2026-10-12). Of Go's platform:admin:function* codes only
-- platform:admin:function:deleted carries the same data as its Java/Rust event
-- (platform:function:function:deleted, {functionId, address}); the use case
-- now emits that code (docs/platform-bug-fixes.md F11). This renames the
-- catalogue row an older seed wrote, so the type keeps its id, status and
-- schema versions, and re-points the subscription bindings that name it: it
-- is the same event under its new code, so a subscription to "Function
-- Deleted" keeps receiving it.
--
-- When the new code is already an event type (this build's seeder, or a Java
-- or Rust seeder on the same database, inserted it first), the bindings move
-- to that row and the old row, a duplicate nothing emits any more, is deleted
-- with its schema versions; no unique-constraint failure. A binding the move
-- would duplicate (its subscription already names the new code) is dropped.
-- Wildcard bindings (platform:admin:function:*) are patterns, not names of
-- this type, and are left alone. Stored events and dispatch jobs keep the
-- code they were written with. Idempotent: once renamed, no old row or
-- binding matches.
--
-- Plain statements with inline VALUES, no temp table (sqlc reads migrations
-- and would model one).
DELETE FROM msg_subscription_event_types b
 USING (VALUES ('platform:admin:function:deleted', 'platform:function:function:deleted')) AS r(old_code, new_code)
 WHERE (b.event_type_code = r.old_code
        OR b.event_type_id IN (SELECT o.id FROM msg_event_types o WHERE o.code = r.old_code))
   AND EXISTS (SELECT 1 FROM msg_subscription_event_types k
                WHERE k.subscription_id = b.subscription_id AND k.event_type_code = r.new_code);

UPDATE msg_subscription_event_types b
   SET event_type_code = r.new_code,
       event_type_id = COALESCE((SELECT n.id FROM msg_event_types n WHERE n.code = r.new_code), b.event_type_id)
  FROM (VALUES ('platform:admin:function:deleted', 'platform:function:function:deleted')) AS r(old_code, new_code)
 WHERE b.event_type_code = r.old_code
    OR b.event_type_id IN (SELECT o.id FROM msg_event_types o WHERE o.code = r.old_code);

DELETE FROM msg_event_type_spec_versions sv
 USING msg_event_types o,
       (VALUES ('platform:admin:function:deleted', 'platform:function:function:deleted')) AS r(old_code, new_code)
 WHERE sv.event_type_id = o.id
   AND o.code = r.old_code
   AND EXISTS (SELECT 1 FROM msg_event_types n WHERE n.code = r.new_code);

DELETE FROM msg_event_types o
 USING (VALUES ('platform:admin:function:deleted', 'platform:function:function:deleted')) AS r(old_code, new_code)
 WHERE o.code = r.old_code
   AND EXISTS (SELECT 1 FROM msg_event_types n WHERE n.code = r.new_code);

UPDATE msg_event_types et
   SET code = r.new_code, subdomain = r.subdomain, aggregate = r.aggregate, updated_at = NOW()
  FROM (VALUES ('platform:admin:function:deleted', 'platform:function:function:deleted', 'function', 'function'))
       AS r(old_code, new_code, subdomain, aggregate)
 WHERE et.code = r.old_code
   AND NOT EXISTS (SELECT 1 FROM msg_event_types n WHERE n.code = r.new_code);

-- +goose Down
-- Renames the row back (bindings follow), only where the old code is free. A
-- duplicate row Up deleted, or a binding it dropped, is not recreated; a build
-- whose seed lists the old code adds the row on its next boot.
UPDATE msg_subscription_event_types b
   SET event_type_code = r.old_code
  FROM (VALUES ('platform:admin:function:deleted', 'platform:function:function:deleted')) AS r(old_code, new_code),
       msg_event_types et
 WHERE et.code = r.new_code
   AND (b.event_type_id = et.id OR b.event_type_code = r.new_code)
   AND NOT EXISTS (SELECT 1 FROM msg_event_types o WHERE o.code = r.old_code);

UPDATE msg_event_types et
   SET code = r.old_code, subdomain = r.subdomain, aggregate = r.aggregate, updated_at = NOW()
  FROM (VALUES ('platform:admin:function:deleted', 'platform:function:function:deleted', 'admin', 'function'))
       AS r(old_code, new_code, subdomain, aggregate)
 WHERE et.code = r.new_code
   AND NOT EXISTS (SELECT 1 FROM msg_event_types o WHERE o.code = r.old_code);
