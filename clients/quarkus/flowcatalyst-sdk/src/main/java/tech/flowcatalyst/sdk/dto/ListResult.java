package tech.flowcatalyst.sdk.dto;

import java.util.List;

/**
 * Generic list result with items and total count.
 */
public record ListResult<T>(
    List<T> items,
    int total
) {
    public static <T> ListResult<T> of(List<T> items) {
        return new ListResult<>(items, items.size());
    }

    public static <T> ListResult<T> empty() {
        return new ListResult<>(List.of(), 0);
    }
}
