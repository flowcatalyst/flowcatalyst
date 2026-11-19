-- +goose Up
-- The install-link permission (docs/app-updates-design.md section 14.7):
-- platform operators create, replace and remove the links through which people
-- download a phone app's binaries. The seeder never rewrites an existing role,
-- so the App Update Administrator role of an existing deployment gets it here,
-- once. An application's own service account does not hold it (the owner's
-- ruling: operators make links, not a client's administrators and not a
-- pipeline).
INSERT INTO iam_role_permissions (role_id, permission)
SELECT r.id, 'platform:app-update:install-link:manage'
  FROM iam_roles r
 WHERE r.name = 'platform:app-update-admin'
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM iam_role_permissions
 WHERE permission = 'platform:app-update:install-link:manage'
   AND role_id IN (SELECT id FROM iam_roles WHERE name = 'platform:app-update-admin');
