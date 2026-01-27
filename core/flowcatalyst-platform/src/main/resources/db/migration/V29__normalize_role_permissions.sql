CREATE TABLE role_permissions (
    role_id VARCHAR(17) NOT NULL REFERENCES auth_roles(id) ON DELETE CASCADE,
    permission VARCHAR(255) NOT NULL,
    PRIMARY KEY (role_id, permission)
);

CREATE INDEX idx_role_permissions_role_id ON role_permissions(role_id);

INSERT INTO role_permissions (role_id, permission)
SELECT id, unnest(permissions)
FROM auth_roles
WHERE permissions IS NOT NULL AND array_length(permissions, 1) > 0;

ALTER TABLE auth_roles DROP COLUMN permissions;
