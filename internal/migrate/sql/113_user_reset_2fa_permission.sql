-- +goose Up
-- platform:iam:user:reset-2fa gates clearing a user's 2FA: the admin
-- reset-2fa action, a send-password-reset that also resets 2FA, and approving
-- a lost-device password reset (whose link clears 2FA). Before it existed,
-- those actions were gated by platform:iam:user:update. So that nobody loses
-- an ability on upgrade, every role that holds user:update is granted
-- reset-2fa here, once — the built-in platform:iam-admin and
-- platform:client-admin (the seeder never rewrites an existing role, so they
-- get it here too), and any custom or application role that held update.
-- From here on the two are separate: a role can be given update without
-- reset-2fa.
--
-- Tokens carry their permissions from mint time: a user must sign in again to
-- hold the new permission.
INSERT INTO iam_role_permissions (role_id, permission)
SELECT rp.role_id, 'platform:iam:user:reset-2fa'
  FROM iam_role_permissions rp
 WHERE rp.permission = 'platform:iam:user:update'
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM iam_role_permissions
 WHERE permission = 'platform:iam:user:reset-2fa';
