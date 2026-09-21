-- Migration 056 PRODUCTION PRE-SCAN
--
-- Run this against PRODUCTION (read-only) BEFORE deploying migration
-- 056_connection_application_scope.sql. That migration replaces the
-- (code, client_id) unique indexes on msg_connections and msg_subscriptions
-- with unique indexes on (application_code, client_id, code) in which "no
-- application" and "no client" are real key values. It pre-checks both tables
-- itself and REFUSES TO APPLY — which stops the platform from starting — if
-- any rows collide under the new key.
--
-- Collisions are possible in real data even though the old index was UNIQUE:
-- Postgres treats NULLs as distinct, so the old index never rejected two rows
-- with the same code and a NULL client_id. Any such pair collides now. (Rows
-- WITH a client_id were genuinely unique before and still are: every existing
-- connection has no application, and an existing subscription's application
-- only ever narrows the key.)
--
-- The grouping is the index expression exactly (an empty string stands for
-- "none" in the output), so nothing can pass this scan and then fail 056.
--
-- EXPECTED RESULT: zero rows. Each row returned is one colliding group, with
-- the ids in it. For each one, decide before deploying which row is the real
-- one, re-point anything that references the others (msg_subscriptions
-- .connection_id for a connection), and rename or remove the rest. Do not
-- deploy 056 until this returns nothing.

-- msg_connections has no application_code column until 056 adds it, and every
-- row it adds starts with none — so before the migration the key is just
-- (client_id, code).
SELECT 'msg_connections' AS table_name,
       ''::varchar AS application_code,
       COALESCE(client_id, '') AS client_id,
       code,
       count(*) AS rows,
       string_agg(id, ', ' ORDER BY created_at) AS ids
  FROM msg_connections
 GROUP BY 3, 4
HAVING count(*) > 1
UNION ALL
SELECT 'msg_subscriptions' AS table_name,
       COALESCE(application_code, '') AS application_code,
       COALESCE(client_id, '') AS client_id,
       code,
       count(*) AS rows,
       string_agg(id, ', ' ORDER BY created_at) AS ids
  FROM msg_subscriptions
 GROUP BY 2, 3, 4
HAVING count(*) > 1
 ORDER BY table_name, code;
