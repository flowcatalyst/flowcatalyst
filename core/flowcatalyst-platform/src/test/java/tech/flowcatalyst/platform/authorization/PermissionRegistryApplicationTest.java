package tech.flowcatalyst.platform.authorization;

import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

import java.util.List;
import java.util.Set;

import static org.junit.jupiter.api.Assertions.*;

/**
 * Unit tests for PermissionRegistry application-aware helper methods.
 */
class PermissionRegistryApplicationTest {

    @Test
    @DisplayName("extractApplicationCode should return first segment")
    void extractApplicationCode_shouldReturnFirstSegment() {
        assertEquals("operant", PermissionRegistry.extractApplicationCode("operant:dispatch:admin"));
        assertEquals("platform", PermissionRegistry.extractApplicationCode("platform:admin"));
        assertEquals("analytics", PermissionRegistry.extractApplicationCode("analytics:viewer"));
    }

    @Test
    @DisplayName("extractApplicationCode should handle edge cases")
    void extractApplicationCode_shouldHandleEdgeCases() {
        assertNull(PermissionRegistry.extractApplicationCode(null));
        assertNull(PermissionRegistry.extractApplicationCode(""));
        assertNull(PermissionRegistry.extractApplicationCode("   "));
        assertEquals("singleword", PermissionRegistry.extractApplicationCode("singleword"));
    }

    @Test
    @DisplayName("getDisplayName should return everything after first colon")
    void getDisplayName_shouldReturnAfterFirstColon() {
        assertEquals("dispatch:admin", PermissionRegistry.getDisplayName("operant:dispatch:admin"));
        assertEquals("admin", PermissionRegistry.getDisplayName("platform:admin"));
        assertEquals("tenant:user:create", PermissionRegistry.getDisplayName("platform:tenant:user:create"));
    }

    @Test
    @DisplayName("getDisplayName should handle edge cases")
    void getDisplayName_shouldHandleEdgeCases() {
        assertNull(PermissionRegistry.getDisplayName(null));
        assertNull(PermissionRegistry.getDisplayName(""));
        assertNull(PermissionRegistry.getDisplayName("   "));
        assertEquals("singleword", PermissionRegistry.getDisplayName("singleword"));
    }

    @Test
    @DisplayName("extractApplicationCodes should return unique codes from roles")
    void extractApplicationCodes_shouldReturnUniqueCodes() {
        Set<String> roles = Set.of(
            "operant:dispatch:admin",
            "operant:dispatch:viewer",
            "platform:admin",
            "analytics:viewer"
        );

        Set<String> appCodes = PermissionRegistry.extractApplicationCodes(roles);

        assertEquals(3, appCodes.size());
        assertTrue(appCodes.contains("operant"));
        assertTrue(appCodes.contains("platform"));
        assertTrue(appCodes.contains("analytics"));
    }

    @Test
    @DisplayName("extractApplicationCodes should handle empty input")
    void extractApplicationCodes_shouldHandleEmptyInput() {
        assertTrue(PermissionRegistry.extractApplicationCodes(null).isEmpty());
        assertTrue(PermissionRegistry.extractApplicationCodes(Set.of()).isEmpty());
        assertTrue(PermissionRegistry.extractApplicationCodes(List.of()).isEmpty());
    }

    @Test
    @DisplayName("filterRolesForApplication should return only matching roles")
    void filterRolesForApplication_shouldReturnMatchingRoles() {
        Set<String> roles = Set.of(
            "operant:dispatch:admin",
            "operant:dispatch:viewer",
            "platform:admin",
            "analytics:viewer"
        );

        Set<String> operantRoles = PermissionRegistry.filterRolesForApplication(roles, "operant");

        assertEquals(2, operantRoles.size());
        assertTrue(operantRoles.contains("operant:dispatch:admin"));
        assertTrue(operantRoles.contains("operant:dispatch:viewer"));
    }

    @Test
    @DisplayName("filterRolesForApplication should return empty for no matches")
    void filterRolesForApplication_shouldReturnEmptyForNoMatches() {
        Set<String> roles = Set.of("operant:admin", "platform:admin");

        Set<String> result = PermissionRegistry.filterRolesForApplication(roles, "unknown");

        assertTrue(result.isEmpty());
    }

    @Test
    @DisplayName("filterRolesForApplication should handle edge cases")
    void filterRolesForApplication_shouldHandleEdgeCases() {
        assertTrue(PermissionRegistry.filterRolesForApplication(null, "app").isEmpty());
        assertTrue(PermissionRegistry.filterRolesForApplication(Set.of(), "app").isEmpty());
        assertTrue(PermissionRegistry.filterRolesForApplication(Set.of("role"), null).isEmpty());
    }
}
