package tech.flowcatalyst.platform.authorization;

import java.lang.annotation.ElementType;
import java.lang.annotation.Retention;
import java.lang.annotation.RetentionPolicy;
import java.lang.annotation.Target;

/**
 * Marker annotation for permission definition classes.
 * Classes annotated with @Permission are scanned at startup and registered in the PermissionRegistry.
 *
 * Example:
 * <pre>
 * @Permission
 * public class MyPermission {
 *     public static final PermissionDefinition INSTANCE = PermissionDefinition.make(
 *         "platform", "tenant", "user", "create", "Create users in tenant"
 *     );
 * }
 * </pre>
 */
@Target(ElementType.TYPE)
@Retention(RetentionPolicy.RUNTIME)
public @interface Permission {
}
