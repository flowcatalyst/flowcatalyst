package tech.flowcatalyst.platform.authentication;

import jakarta.ws.rs.NameBinding;

import java.lang.annotation.ElementType;
import java.lang.annotation.Retention;
import java.lang.annotation.RetentionPolicy;
import java.lang.annotation.Target;

/**
 * Marker annotation for JAX-RS resources and methods that are only available
 * when the auth module is running in EMBEDDED mode.
 *
 * When the application is configured with {@code flowcatalyst.auth.mode=remote},
 * endpoints annotated with this annotation will return 404 Not Found.
 *
 * Usage:
 * <pre>
 * {@literal @}Path("/auth")
 * {@literal @}EmbeddedModeOnly
 * public class AuthResource {
 *     // All endpoints in this class require embedded mode
 * }
 *
 * {@literal @}Path("/api")
 * public class MixedResource {
 *     {@literal @}GET
 *     public Response alwaysAvailable() { ... }
 *
 *     {@literal @}POST
 *     {@literal @}EmbeddedModeOnly
 *     public Response embeddedOnly() { ... }
 * }
 * </pre>
 *
 * @see EmbeddedModeFilter
 * @see AuthMode#EMBEDDED
 */
@NameBinding
@Retention(RetentionPolicy.RUNTIME)
@Target({ElementType.TYPE, ElementType.METHOD})
public @interface EmbeddedModeOnly {
}
