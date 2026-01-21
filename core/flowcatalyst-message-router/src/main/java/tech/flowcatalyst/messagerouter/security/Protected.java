package tech.flowcatalyst.messagerouter.security;

import java.lang.annotation.ElementType;
import java.lang.annotation.Retention;
import java.lang.annotation.RetentionPolicy;
import java.lang.annotation.Target;

/**
 * Marks an endpoint as requiring authentication.
 * Only enforced when authentication is enabled via properties.
 */
@Target({ElementType.TYPE, ElementType.METHOD})
@Retention(RetentionPolicy.RUNTIME)
public @interface Protected {
    /**
     * Description of what this endpoint does (for logging/audit purposes)
     */
    String value() default "";
}
