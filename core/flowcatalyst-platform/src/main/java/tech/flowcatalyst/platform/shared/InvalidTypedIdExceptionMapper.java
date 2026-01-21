package tech.flowcatalyst.platform.shared;

import jakarta.ws.rs.core.MediaType;
import jakarta.ws.rs.core.Response;
import jakarta.ws.rs.ext.ExceptionMapper;
import jakarta.ws.rs.ext.Provider;

import java.util.Map;

/**
 * JAX-RS exception mapper for InvalidTypedIdException.
 *
 * Returns a consistent 400 Bad Request response with details about the invalid ID.
 * This centralizes error handling so individual resources don't need try-catch blocks.
 *
 * Response format:
 * <pre>
 * {
 *   "error": "Invalid client ID format",
 *   "code": "INVALID_ID",
 *   "details": {
 *     "expected_format": "client_{id}",
 *     "provided": "invalid_value"
 *   }
 * }
 * </pre>
 */
@Provider
public class InvalidTypedIdExceptionMapper implements ExceptionMapper<TypedId.InvalidTypedIdException> {

    @Override
    public Response toResponse(TypedId.InvalidTypedIdException exception) {
        var details = Map.of(
            "expected_format", exception.getExpectedType().prefix() + "_<id>",
            "reason", exception.getReason()
        );

        var body = Map.of(
            "error", exception.getMessage(),
            "code", "INVALID_ID",
            "details", details
        );

        return Response.status(Response.Status.BAD_REQUEST)
            .type(MediaType.APPLICATION_JSON)
            .entity(body)
            .build();
    }
}
