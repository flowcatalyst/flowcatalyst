import { bffFetch } from './client';

export type RoleSource = 'CODE' | 'DATABASE' | 'SDK';

export interface Role {
  id: string;
  name: string;
  shortName: string;
  displayName: string;
  description?: string;
  permissions: string[];
  applicationCode: string;
  source: RoleSource;
  clientManaged: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface RoleListResponse {
  items: Role[];
  total: number;
}

export interface ApplicationOption {
  id: string;
  code: string;
  name: string;
}

export interface ApplicationOptionsResponse {
  options: ApplicationOption[];
}

export interface CreateRoleRequest {
  applicationCode: string;
  name: string;
  displayName?: string;
  description?: string;
  permissions?: string[];
  clientManaged?: boolean;
}

export interface UpdateRoleRequest {
  displayName?: string;
  description?: string;
  permissions?: string[];
  clientManaged?: boolean;
}

export interface RoleFilters {
  application?: string;
  source?: RoleSource;
}

export const rolesApi = {
  list(filters: RoleFilters = {}): Promise<RoleListResponse> {
    const params = new URLSearchParams();
    if (filters.application) params.set('application', filters.application);
    if (filters.source) params.set('source', filters.source);

    const query = params.toString();
    return bffFetch(`/roles${query ? `?${query}` : ''}`);
  },

  get(roleName: string): Promise<Role> {
    return bffFetch(`/roles/${encodeURIComponent(roleName)}`);
  },

  create(data: CreateRoleRequest): Promise<Role> {
    return bffFetch('/roles', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  },

  update(roleName: string, data: UpdateRoleRequest): Promise<Role> {
    return bffFetch(`/roles/${encodeURIComponent(roleName)}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  },

  delete(roleName: string): Promise<void> {
    return bffFetch(`/roles/${encodeURIComponent(roleName)}`, { method: 'DELETE' });
  },

  // Filter options
  getApplications(): Promise<ApplicationOptionsResponse> {
    return bffFetch('/roles/filters/applications');
  },
};
