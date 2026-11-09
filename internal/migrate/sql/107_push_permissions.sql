-- +goose Up
-- The push gateway's ingest and sync grants (docs/push-gateway-spec.md sections 6.2 and 8): each
-- application delivers its own outbox (PUSH rows) with its own service-account credentials and
-- syncs its own push rules and ticket keys, so the built-in platform:application-service role
-- carries both. The seeder only inserts a built-in role when it is missing (it never rewrites an
-- existing role's permissions), so existing deployments pick the grants up here, once. Both
-- endpoints confine an application-scoped caller to its own applications.
INSERT INTO iam_role_permissions (role_id, permission)
SELECT r.id, p.permission
  FROM iam_roles r
 CROSS JOIN (VALUES
        ('platform:messaging:batch:push-write'),
        ('platform:application-service:push:sync')
      ) AS p(permission)
 WHERE r.name = 'platform:application-service'
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM iam_role_permissions
 WHERE permission IN (
        'platform:messaging:batch:push-write',
        'platform:application-service:push:sync')
   AND role_id IN (SELECT id FROM iam_roles WHERE name = 'platform:application-service');
