package tech.flowcatalyst.messagerouter.test;

import tech.flowcatalyst.messagerouter.model.MessagePointer;
import tech.flowcatalyst.messagerouter.model.MediationType;

import java.lang.reflect.Field;

/**
 * Shared test utilities to reduce boilerplate and improve test readability.
 *
 * Benefits:
 * - Consistent test data creation
 * - Less verbose test setup
 * - Easy to understand helper methods
 */
public class TestUtils {

    /**
     * Create a test message pointer with sensible defaults
     */
    public static MessagePointer createMessage(String id, String poolCode) {
        return new MessagePointer(id, poolCode, "test-token", MediationType.HTTP, "http://localhost:8080/test", null
            , null);
    }

    /**
     * Create a test message pointer with custom endpoint
     */
    public static MessagePointer createMessage(String id, String poolCode, String endpoint) {
        return new MessagePointer(id, poolCode, "test-token", MediationType.HTTP, endpoint, null
            , null);
    }

    /**
     * Access private field via reflection (use sparingly, only for verification)
     *
     * This is useful for testing internal state, but should be avoided where possible.
     * Prefer using public API or constructor injection for test setup.
     *
     * @param obj The object to access
     * @param fieldName The field name
     * @return The field value
     */
    @SuppressWarnings("unchecked")
    public static <T> T getPrivateField(Object obj, String fieldName) {
        try {
            Field field = obj.getClass().getDeclaredField(fieldName);
            field.setAccessible(true);
            return (T) field.get(obj);
        } catch (Exception e) {
            throw new RuntimeException("Failed to access field: " + fieldName, e);
        }
    }

    /**
     * Set private field via reflection (try to avoid - prefer constructor injection)
     *
     * @param obj The object to modify
     * @param fieldName The field name
     * @param value The value to set
     */
    public static void setPrivateField(Object obj, String fieldName, Object value) {
        try {
            Field field = obj.getClass().getDeclaredField(fieldName);
            field.setAccessible(true);
            field.set(obj, value);
        } catch (Exception e) {
            throw new RuntimeException("Failed to set field: " + fieldName, e);
        }
    }

    /**
     * Sleep without checked exception
     */
    public static void sleep(long millis) {
        try {
            Thread.sleep(millis);
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            throw new RuntimeException("Sleep interrupted", e);
        }
    }
}
