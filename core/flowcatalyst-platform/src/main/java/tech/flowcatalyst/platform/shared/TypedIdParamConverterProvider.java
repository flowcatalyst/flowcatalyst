package tech.flowcatalyst.platform.shared;

import jakarta.inject.Inject;
import jakarta.ws.rs.ext.ParamConverter;
import jakarta.ws.rs.ext.ParamConverterProvider;
import jakarta.ws.rs.ext.Provider;

import java.lang.annotation.Annotation;
import java.lang.reflect.Type;

/**
 * JAX-RS ParamConverterProvider that automatically deserializes typed IDs.
 *
 * When a parameter is annotated with @TypedIdParam, this provider strips
 * the type prefix and returns the raw TSID string. It also validates that
 * the prefix matches the expected entity type.
 *
 * This centralizes typed ID handling so endpoints don't need to manually
 * call typedId.deserialize() for every path parameter.
 */
@Provider
public class TypedIdParamConverterProvider implements ParamConverterProvider {

    @Inject
    TypedId typedId;

    @Override
    @SuppressWarnings("unchecked")
    public <T> ParamConverter<T> getConverter(Class<T> rawType, Type genericType, Annotation[] annotations) {
        // Only handle String parameters
        if (rawType != String.class) {
            return null;
        }

        // Look for @TypedIdParam annotation
        TypedIdParam typedIdParam = null;
        for (Annotation annotation : annotations) {
            if (annotation instanceof TypedIdParam tip) {
                typedIdParam = tip;
                break;
            }
        }

        if (typedIdParam == null) {
            return null;
        }

        EntityType entityType = typedIdParam.value();

        return (ParamConverter<T>) new ParamConverter<String>() {
            @Override
            public String fromString(String value) {
                if (value == null || value.isBlank()) {
                    return null;
                }
                // Validate that the prefix matches expected type, but keep the full ID
                // since IDs are stored WITH prefix in the database (Stripe pattern)
                typedId.validate(entityType, value);
                return value;
            }

            @Override
            public String toString(String value) {
                if (value == null) {
                    return null;
                }
                // Return as-is since IDs already include prefix
                return value;
            }
        };
    }
}
