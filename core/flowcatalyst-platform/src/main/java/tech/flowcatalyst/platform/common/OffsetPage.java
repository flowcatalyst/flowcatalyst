package tech.flowcatalyst.platform.common;

import java.util.List;

/**
 * Generic page record for offset-based pagination.
 *
 * <p>Offset-based pagination is useful when you need:
 * <ul>
 *   <li>Total count for "showing X of Y results" display</li>
 *   <li>Ability to jump to arbitrary page numbers</li>
 * </ul>
 *
 * <p>For large datasets where these aren't needed, prefer cursor-based {@link Page}.
 *
 * @param <T> The type of items in the page
 * @param items The items on this page
 * @param total Total number of items matching the filter (all pages)
 * @param offset The offset used for this page
 * @param limit The page size limit
 */
public record OffsetPage<T>(
    List<T> items,
    long total,
    int offset,
    int limit
) {
    /**
     * Create an empty page with no results.
     */
    public static <T> OffsetPage<T> empty() {
        return new OffsetPage<>(List.of(), 0, 0, 20);
    }

    /**
     * Calculate current page number (0-based).
     */
    public int pageNumber() {
        return limit > 0 ? offset / limit : 0;
    }

    /**
     * Calculate total number of pages.
     */
    public int totalPages() {
        return limit > 0 ? (int) Math.ceil((double) total / limit) : 0;
    }

    /**
     * Check if there are more pages after this one.
     */
    public boolean hasMore() {
        return offset + items.size() < total;
    }

    /**
     * Check if this is the first page.
     */
    public boolean isFirst() {
        return offset == 0;
    }

    /**
     * Check if this is the last page.
     */
    public boolean isLast() {
        return !hasMore();
    }
}
