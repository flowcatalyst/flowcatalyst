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
