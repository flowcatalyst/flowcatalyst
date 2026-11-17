-- +goose Up
-- The app-update permissions (docs/app-updates-design.md section 10).
-- The built-in roles the seeder created before these permissions existed: the
-- seeder never rewrites an existing role, so existing deployments get the
-- grants here, once. An application's service account publishes and rolls
-- back its own application's updates (the handlers confine it to its
-- application); the viewer reads.
INSERT INTO iam_role_permissions (role_id, permission)
SELECT r.id, p.permission
  FROM iam_roles r
 CROSS JOIN (VALUES
        ('platform:app-update:update:view'),
        ('platform:app-update:update:publish'),
        ('platform:app-update:rollout:manage')
      ) AS p(permission)
 WHERE r.name = 'platform:application-service'
ON CONFLICT DO NOTHING;

INSERT INTO iam_role_permissions (role_id, permission)
SELECT r.id, 'platform:app-update:update:view'
  FROM iam_roles r
 WHERE r.name = 'platform:viewer'
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM iam_role_permissions
 WHERE permission IN (
        'platform:app-update:update:view',
        'platform:app-update:update:publish',
        'platform:app-update:rollout:manage')
   AND role_id IN (SELECT id FROM iam_roles WHERE name IN ('platform:application-service', 'platform:viewer'));
