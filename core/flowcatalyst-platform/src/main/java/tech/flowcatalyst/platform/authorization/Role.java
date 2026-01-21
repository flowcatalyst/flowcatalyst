package tech.flowcatalyst.platform.authorization;

import java.lang.annotation.ElementType;
import java.lang.annotation.Retention;
import java.lang.annotation.RetentionPolicy;
import java.lang.annotation.Target;

/**
 * Marker annotation for role definition classes.
 * Classes annotated with @Role are scanned at startup and registered in the PermissionRegistry.
 *
 * Example:
 * <pre>
 * @Role
 * public class MyRole {
 *     public static final RoleDefinition INSTANCE = RoleDefinition.make(
 *         "platform",
 *         "tenant-admin",
 *         Set.of(
 *             PlatformTenantUserCreatePermission.INSTANCE,
 *             PlatformTenantUserViewPermission.INSTANCE
 *         ),
 *         "Tenant administrator role"
 *     );
 * }
 * </pre>
 */
@Target(ElementType.TYPE)
@Retention(RetentionPolicy.RUNTIME)
public @interface Role {
}
