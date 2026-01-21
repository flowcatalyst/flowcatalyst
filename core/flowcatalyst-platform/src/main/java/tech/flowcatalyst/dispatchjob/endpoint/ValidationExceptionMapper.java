package tech.flowcatalyst.dispatchjob.endpoint;

import jakarta.validation.ConstraintViolation;
import jakarta.validation.ConstraintViolationException;
import jakarta.ws.rs.core.Response;
import jakarta.ws.rs.ext.ExceptionMapper;
import jakarta.ws.rs.ext.Provider;
import tech.flowcatalyst.dispatchjob.dto.ErrorResponse;

import java.util.stream.Collectors;

@Provider
public class ValidationExceptionMapper implements ExceptionMapper<ConstraintViolationException> {

    @Override
    public Response toResponse(ConstraintViolationException exception) {
        String errorMessage = exception.getConstraintViolations()
            .stream()
            .map(ConstraintViolation::getMessage)
            .collect(Collectors.joining(", "));

        return Response
            .status(Response.Status.BAD_REQUEST)
            .entity(new ErrorResponse(errorMessage))
            .build();
    }
}
