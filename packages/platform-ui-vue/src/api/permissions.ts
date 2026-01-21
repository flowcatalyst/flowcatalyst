import { bffFetch } from './client';

export interface Permission {
  permission: string;
  application: string;
  context: string;
  aggregate: string;
  action: string;
  description: string;
}

export interface PermissionListResponse {
  items: Permission[];
  total: number;
}

export const permissionsApi = {
  list(): Promise<PermissionListResponse> {
    return bffFetch('/roles/permissions');
  },

  get(permission: string): Promise<Permission> {
    return bffFetch(`/roles/permissions/${encodeURIComponent(permission)}`);
  },
};
