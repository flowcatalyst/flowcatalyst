package tech.flowcatalyst.sdk.exception;

import java.util.List;
import java.util.Map;

/**
 * Exception thrown when validation fails.
 */
public class ValidationException extends FlowCatalystException {

    private final List<ValidationError> errors;

    public ValidationException(String message, List<ValidationError> errors) {
        super(message, 422);
        this.errors = errors;
    }

    public List<ValidationError> getErrors() {
        return errors;
    }

    public static ValidationException fromResponse(Map<String, Object> response) {
        @SuppressWarnings("unchecked")
        var errorList = (List<Map<String, Object>>) response.getOrDefault("errors", List.of());

        var errors = errorList.stream()
            .map(e -> new ValidationError(
                (String) e.get("field"),
                (String) e.get("message"),
                (String) e.get("code")
            ))
            .toList();

        String message = (String) response.getOrDefault("error", "Validation failed");
        return new ValidationException(message, errors);
    }

    public record ValidationError(String field, String message, String code) {}
}
