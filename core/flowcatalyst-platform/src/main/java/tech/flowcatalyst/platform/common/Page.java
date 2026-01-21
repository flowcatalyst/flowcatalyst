package tech.flowcatalyst.platform.common;

import java.util.List;

/**
 * Generic page record for cursor-based pagination.
 *
 * <p>Cursor-based pagination is more efficient than offset-based pagination
 * for large datasets because it doesn't require counting or skipping rows.
 *
 * <p>Usage pattern:
 * <pre>{@code
 * Page<Event> page = eventRepository.findPage(null, 50);  // First page
 * while (page.hasMore()) {
 *     process(page.items());
 *     page = eventRepository.findPage(page.nextCursor(), 50);
 * }
 * }</pre>
 *
 * @param <T> The type of items in the page
 * @param items The items on this page
 * @param nextCursor The cursor to use to fetch the next page, or null if no more pages
 * @param hasMore Whether there are more items after this page
 */
public record Page<T>(
    List<T> items,
    String nextCursor,
    boolean hasMore
) {
    /**
     * Create an empty page with no more results.
     */
    public static <T> Page<T> empty() {
        return new Page<>(List.of(), null, false);
    }

    /**
     * Create a page from a list of items, determining if there are more based on
     * whether we fetched more items than the requested limit.
     *
     * @param items The items fetched (may include one extra to detect hasMore)
     * @param limit The requested page size
     * @param cursorExtractor Function to extract cursor from the last item
     * @param <T> The item type
     * @return A Page with proper hasMore and nextCursor set
     */
    public static <T> Page<T> of(List<T> items, int limit, java.util.function.Function<T, String> cursorExtractor) {
        if (items == null || items.isEmpty()) {
            return empty();
        }

        boolean hasMore = items.size() > limit;
        List<T> pageItems = hasMore ? items.subList(0, limit) : items;
        String cursor = hasMore && !pageItems.isEmpty()
            ? cursorExtractor.apply(pageItems.get(pageItems.size() - 1))
            : null;

        return new Page<>(pageItems, cursor, hasMore);
    }
}
