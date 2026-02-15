/**
 * Platform Admin Permissions
 *
 * Permissions for platform administration operations.
 */

import { makePermission, type PermissionDefinition } from '../permission-definition.js';

const SUBDOMAIN = 'platform';
const CONTEXT = 'admin';

/**
 * Client permissions.
 */
export const CLIENT_PERMISSIONS = {
  CREATE: makePermission(SUBDOMAIN, CONTEXT, 'client', 'create', 'Create clients'),
  READ: makePermission(SUBDOMAIN, CONTEXT, 'client', 'read', 'Read client details'),
  UPDATE: makePermission(SUBDOMAIN, CONTEXT, 'client', 'update', 'Update client details'),
  DELETE: makePermission(SUBDOMAIN, CONTEXT, 'client', 'delete', 'Delete clients'),
  MANAGE: makePermission(SUBDOMAIN, CONTEXT, 'client', 'manage', 'Full client management'),
  ACTIVATE: makePermission(SUBDOMAIN, CONTEXT, 'client', 'activate', 'Activate clients'),
  SUSPEND: makePermission(SUBDOMAIN, CONTEXT, 'client', 'suspend', 'Suspend clients'),
  DEACTIVATE: makePermission(SUBDOMAIN, CONTEXT, 'client', 'deactivate', 'Deactivate clients'),
} as const;

/**
 * Anchor domain permissions.
 */
export const ANCHOR_DOMAIN_PERMISSIONS = {
  CREATE: makePermission(SUBDOMAIN, CONTEXT, 'anchor-domain', 'create', 'Create anchor domains'),
  READ: makePermission(SUBDOMAIN, CONTEXT, 'anchor-domain', 'read', 'Read anchor domains'),
  UPDATE: makePermission(SUBDOMAIN, CONTEXT, 'anchor-domain', 'update', 'Update anchor domains'),
  DELETE: makePermission(SUBDOMAIN, CONTEXT, 'anchor-domain', 'delete', 'Delete anchor domains'),
  MANAGE: makePermission(
    SUBDOMAIN,
    CONTEXT,
    'anchor-domain',
    'manage',
    'Full anchor domain management',
  ),
} as const;

/**
 * Application permissions.
 */
export const APPLICATION_PERMISSIONS = {
  CREATE: makePermission(SUBDOMAIN, CONTEXT, 'application', 'create', 'Create applications'),
  READ: makePermission(SUBDOMAIN, CONTEXT, 'application', 'read', 'Read application details'),
  UPDATE: makePermission(SUBDOMAIN, CONTEXT, 'application', 'update', 'Update application details'),
  DELETE: makePermission(SUBDOMAIN, CONTEXT, 'application', 'delete', 'Delete applications'),
  MANAGE: makePermission(
    SUBDOMAIN,
    CONTEXT,
    'application',
    'manage',
    'Full application management',
  ),
  ACTIVATE: makePermission(SUBDOMAIN, CONTEXT, 'application', 'activate', 'Activate applications'),
  DEACTIVATE: makePermission(
    SUBDOMAIN,
    CONTEXT,
    'application',
    'deactivate',
    'Deactivate applications',
  ),
  ENABLE_CLIENT: makePermission(
    SUBDOMAIN,
    CONTEXT,
    'application',
    'enable-client',
    'Enable application for client',
  ),
  DISABLE_CLIENT: makePermission(
    SUBDOMAIN,
    CONTEXT,
    'application',
    'disable-client',
    'Disable application for client',
  ),
} as const;

/**
 * Event type permissions.
 */
export const EVENT_TYPE_PERMISSIONS = {
  CREATE: makePermission(SUBDOMAIN, CONTEXT, 'event-type', 'create', 'Create event types'),
  READ: makePermission(SUBDOMAIN, CONTEXT, 'event-type', 'read', 'Read event type details'),
  UPDATE: makePermission(SUBDOMAIN, CONTEXT, 'event-type', 'update', 'Update event type details'),
  DELETE: makePermission(SUBDOMAIN, CONTEXT, 'event-type', 'delete', 'Delete event types'),
  MANAGE: makePermission(SUBDOMAIN, CONTEXT, 'event-type', 'manage', 'Full event type management'),
  ARCHIVE: makePermission(SUBDOMAIN, CONTEXT, 'event-type', 'archive', 'Archive event types'),
  MANAGE_SCHEMA: makePermission(
    SUBDOMAIN,
    CONTEXT,
    'event-type',
    'manage-schema',
    'Manage event type schemas',
  ),
  SYNC: makePermission(SUBDOMAIN, CONTEXT, 'event-type', 'sync', 'Sync event types from SDK'),
} as const;

/**
 * Dispatch pool permissions.
 */
export const DISPATCH_POOL_PERMISSIONS = {
  CREATE: makePermission(SUBDOMAIN, CONTEXT, 'dispatch-pool', 'create', 'Create dispatch pools'),
  READ: makePermission(SUBDOMAIN, CONTEXT, 'dispatch-pool', 'read', 'Read dispatch pool details'),
  UPDATE: makePermission(
    SUBDOMAIN,
    CONTEXT,
    'dispatch-pool',
    'update',
    'Update dispatch pool details',
  ),
  DELETE: makePermission(SUBDOMAIN, CONTEXT, 'dispatch-pool', 'delete', 'Delete dispatch pools'),
  MANAGE: makePermission(
    SUBDOMAIN,
    CONTEXT,
    'dispatch-pool',
    'manage',
    'Full dispatch pool management',
  ),
  SYNC: makePermission(SUBDOMAIN, CONTEXT, 'dispatch-pool', 'sync', 'Sync dispatch pools from SDK'),
} as const;

/**
 * Subscription permissions.
 */
export const SUBSCRIPTION_PERMISSIONS = {
  CREATE: makePermission(SUBDOMAIN, CONTEXT, 'subscription', 'create', 'Create subscriptions'),
  READ: makePermission(SUBDOMAIN, CONTEXT, 'subscription', 'read', 'Read subscription details'),
  UPDATE: makePermission(
    SUBDOMAIN,
    CONTEXT,
    'subscription',
    'update',
    'Update subscription details',
  ),
  DELETE: makePermission(SUBDOMAIN, CONTEXT, 'subscription', 'delete', 'Delete subscriptions'),
  MANAGE: makePermission(
    SUBDOMAIN,
    CONTEXT,
    'subscription',
    'manage',
    'Full subscription management',
  ),
  SYNC: makePermission(SUBDOMAIN, CONTEXT, 'subscription', 'sync', 'Sync subscriptions from SDK'),
} as const;

/**
 * Event read permissions (BFF).
 */
export const EVENT_PERMISSIONS = {
  READ: makePermission(SUBDOMAIN, CONTEXT, 'event', 'read', 'Read events'),
  VIEW_RAW: makePermission(SUBDOMAIN, CONTEXT, 'event', 'view-raw', 'View raw events (debug)'),
} as const;

/**
 * Dispatch job read permissions (BFF).
 */
export const DISPATCH_JOB_PERMISSIONS = {
  READ: makePermission(SUBDOMAIN, CONTEXT, 'dispatch-job', 'read', 'Read dispatch jobs'),
  VIEW_RAW: makePermission(
    SUBDOMAIN,
    CONTEXT,
    'dispatch-job',
    'view-raw',
    'View raw dispatch jobs (debug)',
  ),
} as const;

/**
 * Identity provider permissions.
 */
export const IDENTITY_PROVIDER_PERMISSIONS = {
  CREATE: makePermission(
    SUBDOMAIN,
    CONTEXT,
    'identity-provider',
    'create',
    'Create identity providers',
  ),
  READ: makePermission(SUBDOMAIN, CONTEXT, 'identity-provider', 'read', 'Read identity providers'),
  UPDATE: makePermission(
    SUBDOMAIN,
    CONTEXT,
    'identity-provider',
    'update',
    'Update identity providers',
  ),
  DELETE: makePermission(
    SUBDOMAIN,
    CONTEXT,
    'identity-provider',
    'delete',
    'Delete identity providers',
  ),
  MANAGE: makePermission(
    SUBDOMAIN,
    CONTEXT,
    'identity-provider',
    'manage',
    'Full identity provider management',
  ),
} as const;

/**
 * Email domain mapping permissions.
 */
export const EMAIL_DOMAIN_MAPPING_PERMISSIONS = {
  CREATE: makePermission(
    SUBDOMAIN,
    CONTEXT,
    'email-domain-mapping',
    'create',
    'Create email domain mappings',
  ),
  READ: makePermission(
    SUBDOMAIN,
    CONTEXT,
    'email-domain-mapping',
    'read',
    'Read email domain mappings',
  ),
  UPDATE: makePermission(
    SUBDOMAIN,
    CONTEXT,
    'email-domain-mapping',
    'update',
    'Update email domain mappings',
  ),
  DELETE: makePermission(
    SUBDOMAIN,
    CONTEXT,
    'email-domain-mapping',
    'delete',
    'Delete email domain mappings',
  ),
  MANAGE: makePermission(
    SUBDOMAIN,
    CONTEXT,
    'email-domain-mapping',
    'manage',
    'Full email domain mapping management',
  ),
} as const;

/**
 * Service account permissions.
 */
export const SERVICE_ACCOUNT_PERMISSIONS = {
  CREATE: makePermission(
    SUBDOMAIN,
    CONTEXT,
    'service-account',
    'create',
    'Create service accounts',
  ),
  READ: makePermission(SUBDOMAIN, CONTEXT, 'service-account', 'read', 'Read service accounts'),
  UPDATE: makePermission(
    SUBDOMAIN,
    CONTEXT,
    'service-account',
    'update',
    'Update service accounts',
  ),
  DELETE: makePermission(
    SUBDOMAIN,
    CONTEXT,
    'service-account',
    'delete',
    'Delete service accounts',
  ),
  MANAGE: makePermission(
    SUBDOMAIN,
    CONTEXT,
    'service-account',
    'manage',
    'Full service account management',
  ),
} as const;

/**
 * CORS origin permissions.
 */
export const CORS_ORIGIN_PERMISSIONS = {
  CREATE: makePermission(SUBDOMAIN, CONTEXT, 'cors-origin', 'create', 'Create CORS origins'),
  READ: makePermission(SUBDOMAIN, CONTEXT, 'cors-origin', 'read', 'Read CORS origins'),
  DELETE: makePermission(SUBDOMAIN, CONTEXT, 'cors-origin', 'delete', 'Delete CORS origins'),
  MANAGE: makePermission(
    SUBDOMAIN,
    CONTEXT,
    'cors-origin',
    'manage',
    'Full CORS origin management',
  ),
} as const;

/**
 * Audit log permissions.
 */
export const AUDIT_LOG_PERMISSIONS = {
  READ: makePermission(SUBDOMAIN, CONTEXT, 'audit-log', 'read', 'Read audit logs'),
  EXPORT: makePermission(SUBDOMAIN, CONTEXT, 'audit-log', 'export', 'Export audit logs'),
} as const;

/**
 * Batch ingestion permissions (outbox processor / SDK batch endpoints).
 */
export const BATCH_PERMISSIONS = {
  EVENTS_WRITE: makePermission(SUBDOMAIN, CONTEXT, 'batch', 'events-write', 'Batch write events'),
  DISPATCH_JOBS_WRITE: makePermission(
    SUBDOMAIN,
    CONTEXT,
    'batch',
    'dispatch-jobs-write',
    'Batch write dispatch jobs',
  ),
  AUDIT_LOGS_WRITE: makePermission(
    SUBDOMAIN,
    CONTEXT,
    'batch',
    'audit-logs-write',
    'Batch write audit logs',
  ),
} as const;

/**
 * All admin permissions.
 */
export const ADMIN_PERMISSIONS: readonly PermissionDefinition[] = [
  ...Object.values(CLIENT_PERMISSIONS),
  ...Object.values(ANCHOR_DOMAIN_PERMISSIONS),
  ...Object.values(APPLICATION_PERMISSIONS),
  ...Object.values(EVENT_TYPE_PERMISSIONS),
  ...Object.values(DISPATCH_POOL_PERMISSIONS),
  ...Object.values(SUBSCRIPTION_PERMISSIONS),
  ...Object.values(EVENT_PERMISSIONS),
  ...Object.values(DISPATCH_JOB_PERMISSIONS),
  ...Object.values(IDENTITY_PROVIDER_PERMISSIONS),
  ...Object.values(EMAIL_DOMAIN_MAPPING_PERMISSIONS),
  ...Object.values(SERVICE_ACCOUNT_PERMISSIONS),
  ...Object.values(CORS_ORIGIN_PERMISSIONS),
  ...Object.values(AUDIT_LOG_PERMISSIONS),
  ...Object.values(BATCH_PERMISSIONS),
];
