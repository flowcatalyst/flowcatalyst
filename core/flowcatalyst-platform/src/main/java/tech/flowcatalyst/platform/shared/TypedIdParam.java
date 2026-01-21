package tech.flowcatalyst.platform.shared;

import java.lang.annotation.ElementType;
import java.lang.annotation.Retention;
import java.lang.annotation.RetentionPolicy;
import java.lang.annotation.Target;

/**
 * Annotation for JAX-RS path/query parameters that contain typed IDs.
 *
 * When applied to a parameter, the TypedIdParamConverterProvider will
 * automatically strip the type prefix (e.g., "client_", "app_") and
 * return the raw TSID string.
 *
 * Example usage:
 * <pre>
 * @GET
 * @Path("/{id}")
 * public Response getClient(@TypedIdParam(EntityType.CLIENT) @PathParam("id") String id) {
 *     // id is now the raw TSID without prefix
 * }
 * </pre>
 */
@Target({ElementType.PARAMETER, ElementType.FIELD})
@Retention(RetentionPolicy.RUNTIME)
public @interface TypedIdParam {
    /**
     * The expected entity type for this ID parameter.
     */
    EntityType value();
}
