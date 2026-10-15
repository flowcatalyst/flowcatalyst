-- +goose Up
-- The built-in platform:application-service role now carries
-- platform:messaging:dispatch-pool:sync so an application's deploy can sync its
-- own dispatch pools (docs/platform-bug-fixes.md F19). The seeder only inserts a
-- built-in role when it is missing (it never rewrites an existing role's
-- permissions), so existing deployments pick the grant up here, once. The sync
-- confines an application-scoped caller to its bound applications and to pools
-- that application owns (migration 078).
--
-- Tokens carry their permissions from mint time: an application service
-- account must mint a new token to hold the new permission.
INSERT INTO iam_role_permissions (role_id, permission)
SELECT r.id, 'platform:messaging:dispatch-pool:sync'
  FROM iam_roles r
 WHERE r.name = 'platform:application-service'
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM iam_role_permissions
 WHERE permission = 'platform:messaging:dispatch-pool:sync'
   AND role_id IN (SELECT id FROM iam_roles WHERE name = 'platform:application-service');
