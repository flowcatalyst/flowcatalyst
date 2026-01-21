import { defineStore } from 'pinia';
import { ref, computed } from 'vue';

/**
 * Permission denied event payload
 */
export interface PermissionDeniedEvent {
  type: 'api' | 'route';
  message: string;
  requiredPermission?: string;
  path?: string;
}

export const usePermissionsStore = defineStore('permissions', () => {
  // State
  const userPermissions = ref<string[]>([]);
  const permissionDenied = ref<PermissionDeniedEvent | null>(null);
  const showPermissionModal = ref(false);

  // Computed
  const hasPermission = computed(() => (permission: string) => {
    // Super admin check - if user has platform:super-admin role, they have all permissions
    // This is handled server-side, but we can also check for wildcard
    if (userPermissions.value.includes('*')) {
      return true;
    }
    return userPermissions.value.includes(permission);
  });

  // Actions
  function setPermissions(permissions: string[]) {
    userPermissions.value = permissions;
  }

  function clearPermissions() {
    userPermissions.value = [];
  }

  function showPermissionDenied(event: PermissionDeniedEvent) {
    permissionDenied.value = event;
    showPermissionModal.value = true;
  }

  function hidePermissionDenied() {
    showPermissionModal.value = false;
    // Clear after animation
    setTimeout(() => {
      permissionDenied.value = null;
    }, 300);
  }

  function handleApiError(status: number, message?: string) {
    if (status === 401) {
      showPermissionDenied({
        type: 'api',
        message: 'Your session has expired. Please log in again.',
      });
    } else if (status === 403) {
      showPermissionDenied({
        type: 'api',
        message: message || 'You do not have permission to perform this action.',
      });
    }
  }

  return {
    // State
    userPermissions,
    permissionDenied,
    showPermissionModal,
    // Computed
    hasPermission,
    // Actions
    setPermissions,
    clearPermissions,
    showPermissionDenied,
    hidePermissionDenied,
    handleApiError,
  };
});

/**
 * Route permission requirements mapping.
 * Maps route paths to required permissions.
 */
export const ROUTE_PERMISSIONS: Record<string, string> = {
  // Applications
  '/applications': 'platform:admin:application:view',
  '/applications/new': 'platform:admin:application:create',

  // Clients
  '/clients': 'platform:admin:client:view',
  '/clients/new': 'platform:admin:client:create',

  // Users
  '/users': 'platform:iam:user:view',
  '/users/new': 'platform:iam:user:create',

  // Authorization
  '/authorization/roles': 'platform:iam:role:view',
  '/authorization/permissions': 'platform:iam:permission:view',

  // Authentication
  '/authentication/domain-idps': 'platform:iam:idp:manage',
  '/authentication/anchor-domains': 'platform:iam:idp:manage',

  // Event Types
  '/event-types': 'platform:messaging:event-type:view',
  '/event-types/create': 'platform:messaging:event-type:create',

  // Subscriptions
  '/subscriptions': 'platform:messaging:subscription:view',
  '/subscriptions/new': 'platform:messaging:subscription:create',

  // Dispatch Pools
  '/dispatch-pools': 'platform:messaging:dispatch-pool:view',
  '/dispatch-pools/new': 'platform:messaging:dispatch-pool:create',

  // Dispatch Jobs
  '/dispatch-jobs': 'platform:messaging:dispatch-job:view',

  // Audit Log
  '/platform/audit-log': 'platform:admin:audit:view',
};

/**
 * Get the required permission for a route path.
 * Handles dynamic routes like /applications/:id
 */
export function getRoutePermission(path: string): string | undefined {
  // First check exact match
  if (ROUTE_PERMISSIONS[path]) {
    return ROUTE_PERMISSIONS[path];
  }

  // Check for dynamic routes (e.g., /applications/123 -> /applications)
  // Handle detail pages - they typically need view permission
  const segments = path.split('/').filter(Boolean);
  if (segments.length >= 2) {
    // Check if last segment looks like an ID (not a keyword like 'new' or 'create')
    const lastSegment = segments[segments.length - 1];
    if (lastSegment !== 'new' && lastSegment !== 'create' && lastSegment !== 'edit') {
      // Try base path
      const basePath = '/' + segments.slice(0, -1).join('/');
      if (ROUTE_PERMISSIONS[basePath]) {
        return ROUTE_PERMISSIONS[basePath];
      }
    }
  }

  return undefined;
}
