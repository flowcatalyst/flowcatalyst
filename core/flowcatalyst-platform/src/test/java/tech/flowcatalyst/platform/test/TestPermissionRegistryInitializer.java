package tech.flowcatalyst.platform.test;

import io.quarkus.runtime.StartupEvent;
import io.quarkus.logging.Log;
import jakarta.annotation.Priority;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.event.Observes;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.authorization.PermissionRegistry;

/**
 * Test-specific initializer that registers test permissions and roles.
 * This runs during test startup AFTER PermissionRegistry initialization.
 */
@ApplicationScoped
public class TestPermissionRegistryInitializer {

    @Inject
    PermissionRegistry registry;

    void onStart(@Observes @Priority(100) StartupEvent event) {
        Log.info("Registering test permissions and roles...");

        // Register test permissions first
        // Note: Duplicates are automatically skipped by the registry
        registry.registerPermission(TestContextResourceViewPermission.INSTANCE);
        registry.registerPermission(TestContextResourceCreatePermission.INSTANCE);
        registry.registerPermission(TestContextResourceUpdatePermission.INSTANCE);
        registry.registerPermission(TestContextResourceDeletePermission.INSTANCE);
        registry.registerPermission(PlatformClientUserViewTestPermission.INSTANCE);
        registry.registerPermission(PlatformClientUserCreateTestPermission.INSTANCE);
        registry.registerPermission(PlatformClientManageTestPermission.INSTANCE);

        // Register test roles (must be after permissions)
        registry.registerRole(TestViewerRole.INSTANCE);
        registry.registerRole(TestEditorRole.INSTANCE);
        registry.registerRole(TestAdminRole.INSTANCE);
        registry.registerRole(PlatformTestClientAdminRole.INSTANCE);

        Log.info("Test permissions and roles registered successfully");
    }
}
