/**
 * API client for Audit Log operations.
 */

import { apiFetch } from './client';

export interface AuditLog {
  id: string;
  entityType: string;
  entityId: string;
  operation: string;
  principalId: string | null;
  principalName: string | null;
  performedAt: string;
}

export interface AuditLogDetail extends AuditLog {
  operationJson: string | null;
}

export interface AuditLogListResponse {
  auditLogs: AuditLog[];
  total: number;
  page: number;
  pageSize: number;
}

export interface AuditLogFilters {
  entityType?: string;
  entityId?: string;
  principalId?: string;
  operation?: string;
  page?: number;
  pageSize?: number;
}

/**
 * Fetch audit logs with optional filters and pagination.
 */
export async function fetchAuditLogs(filters: AuditLogFilters = {}): Promise<AuditLogListResponse> {
  const params = new URLSearchParams();
  if (filters.entityType) params.set('entityType', filters.entityType);
  if (filters.entityId) params.set('entityId', filters.entityId);
  if (filters.principalId) params.set('principalId', filters.principalId);
  if (filters.operation) params.set('operation', filters.operation);
  if (filters.page !== undefined) params.set('page', String(filters.page));
  if (filters.pageSize !== undefined) params.set('pageSize', String(filters.pageSize));

  const query = params.toString();
  return apiFetch<AuditLogListResponse>(`/admin/audit-logs${query ? `?${query}` : ''}`);
}

/**
 * Fetch a single audit log by ID.
 */
export async function fetchAuditLogById(id: string): Promise<AuditLogDetail> {
  return apiFetch<AuditLogDetail>(`/admin/audit-logs/${id}`);
}

/**
 * Fetch audit logs for a specific entity.
 */
export async function fetchAuditLogsForEntity(
  entityType: string,
  entityId: string
): Promise<AuditLogListResponse> {
  return apiFetch<AuditLogListResponse>(
    `/admin/audit-logs/entity/${encodeURIComponent(entityType)}/${encodeURIComponent(entityId)}`
  );
}

/**
 * Fetch distinct entity types that have audit logs.
 */
export async function fetchEntityTypes(): Promise<{ entityTypes: string[] }> {
  return apiFetch<{ entityTypes: string[] }>('/admin/audit-logs/entity-types');
}

/**
 * Fetch distinct operations that have audit logs.
 */
export async function fetchOperations(): Promise<{ operations: string[] }> {
  return apiFetch<{ operations: string[] }>('/admin/audit-logs/operations');
}
