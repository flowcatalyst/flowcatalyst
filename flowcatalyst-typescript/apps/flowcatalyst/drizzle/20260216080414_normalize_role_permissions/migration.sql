-- 1. Create role_permissions junction table
CREATE TABLE "role_permissions" (
	"role_id" varchar(17) NOT NULL,
	"permission" varchar(255) NOT NULL,
	CONSTRAINT "role_permissions_role_id_permission_pk" PRIMARY KEY("role_id","permission")
);
--> statement-breakpoint
ALTER TABLE "role_permissions" ADD CONSTRAINT "role_permissions_role_id_auth_roles_id_fk" FOREIGN KEY ("role_id") REFERENCES "public"."auth_roles"("id") ON DELETE cascade ON UPDATE no action;
--> statement-breakpoint
CREATE INDEX "idx_role_permissions_role_id" ON "role_permissions" ("role_id");
--> statement-breakpoint

-- 2. Migrate existing JSONB permissions data to junction table
INSERT INTO role_permissions (role_id, permission)
SELECT id, jsonb_array_elements_text(permissions)
FROM auth_roles
WHERE permissions IS NOT NULL AND jsonb_array_length(permissions) > 0;
--> statement-breakpoint

-- 3. Drop the permissions JSONB column from auth_roles
ALTER TABLE "auth_roles" DROP COLUMN "permissions";
