-- Queries for iam_roles + iam_role_permissions. Permissions are
-- many-to-many; Persist replaces them wholesale.

-- name: RoleFindByID :one
SELECT id, application_id, application_code, name, display_name, description,
       source, client_managed, created_at, updated_at
FROM iam_roles
WHERE id = $1;

-- name: RoleFindByName :one
SELECT id, application_id, application_code, name, display_name, description,
       source, client_managed, created_at, updated_at
FROM iam_roles
WHERE name = $1;

-- name: RoleFindByShortNameInApps :one
-- Resolve a role by its UNPREFIXED short name within a set of applications.
-- SDK-synced principal role assignments store the bare role name (e.g.
-- "hr-manager") rather than the canonical iam_roles.name ("hr:hr-manager"), so
-- an exact name match misses. name = application_code || ':' || <roleName> is
-- the canonical form built by role.New, so this match is exact (no fuzzy
-- suffix matching) and scoped to the given applications.
SELECT id, application_id, application_code, name, display_name, description,
       source, client_managed, created_at, updated_at
FROM iam_roles
WHERE application_id = ANY(@app_ids::text[])
  AND name = application_code || ':' || @short_name::text
LIMIT 1;

-- name: RoleFindAll :many
SELECT id, application_id, application_code, name, display_name, description,
       source, client_managed, created_at, updated_at
FROM iam_roles
ORDER BY name;

-- name: RoleFindBySource :many
SELECT id, application_id, application_code, name, display_name, description,
       source, client_managed, created_at, updated_at
FROM iam_roles
WHERE source = $1
ORDER BY name;

-- name: RoleCountAssignments :one
SELECT COUNT(*) FROM iam_principal_roles WHERE role_name = $1;

-- name: RoleUpsert :exec
INSERT INTO iam_roles
    (id, application_id, name, display_name, description, application_code,
     source, client_managed, created_at, updated_at)
VALUES
    ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (id) DO UPDATE SET
    application_id = EXCLUDED.application_id,
    name = EXCLUDED.name,
    display_name = EXCLUDED.display_name,
    description = EXCLUDED.description,
    application_code = EXCLUDED.application_code,
    source = EXCLUDED.source,
    client_managed = EXCLUDED.client_managed,
    updated_at = EXCLUDED.updated_at;

-- name: RoleDelete :exec
DELETE FROM iam_roles WHERE id = $1;

-- name: RolePermissionsClear :exec
DELETE FROM iam_role_permissions WHERE role_id = $1;

-- name: RolePermissionInsert :exec
INSERT INTO iam_role_permissions (role_id, permission)
VALUES (@role_id, @permission);

-- name: RolePermissionsForRoles :many
SELECT role_id, permission
FROM iam_role_permissions
WHERE role_id = ANY(@role_ids::text[]);

-- name: RoleFindByApplicationID :many
SELECT id, application_id, application_code, name, display_name, description,
       source, client_managed, created_at, updated_at
FROM iam_roles
WHERE application_id = $1
ORDER BY name;

-- name: RoleApplicationCodes :many
SELECT DISTINCT application_code FROM iam_roles ORDER BY application_code;

-- name: PermissionFindAll :many
SELECT id, code, subdomain, context, aggregate, action, description, created_at, updated_at
FROM iam_permissions
ORDER BY code;

-- name: PermissionFindByCode :one
SELECT id, code, subdomain, context, aggregate, action, description, created_at, updated_at
FROM iam_permissions
WHERE code = $1;

-- name: PermissionUpsert :exec
INSERT INTO iam_permissions (id, code, subdomain, context, aggregate, action, description)
VALUES (@id, @code, @subdomain, @context, @aggregate, @action, @description)
ON CONFLICT (code) DO UPDATE SET
    subdomain   = EXCLUDED.subdomain,
    context     = EXCLUDED.context,
    aggregate   = EXCLUDED.aggregate,
    action      = EXCLUDED.action,
    description = EXCLUDED.description,
    updated_at  = NOW();

-- name: PermissionDeleteByCode :exec
DELETE FROM iam_permissions WHERE code = $1;
