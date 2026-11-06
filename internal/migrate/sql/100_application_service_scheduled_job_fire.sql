-- +goose Up
-- An application's service account may look up and fire its own applications'
-- scheduled jobs, so the built-in platform:application-service role now carries
-- platform:application-service:scheduled-job:fire. The seeder only inserts a
-- built-in role when it is missing (it never rewrites an existing role's
-- permissions), so existing deployments pick the grant up here, once. The
-- scheduled-job endpoints confine an application-only caller to jobs of the
-- applications it is bound to.
INSERT INTO iam_role_permissions (role_id, permission)
SELECT r.id, 'platform:application-service:scheduled-job:fire'
  FROM iam_roles r
 WHERE r.name = 'platform:application-service'
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM iam_role_permissions
 WHERE permission = 'platform:application-service:scheduled-job:fire'
   AND role_id IN (SELECT id FROM iam_roles WHERE name = 'platform:application-service');
