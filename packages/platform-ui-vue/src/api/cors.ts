import { apiFetch } from './client';

export interface CorsOrigin {
  id: string;
  origin: string;
  description: string | null;
  createdBy: string;
  createdAt: string;
}

export interface CorsOriginListResponse {
  items: CorsOrigin[];
  total: number;
}

export interface CreateCorsOriginRequest {
  origin: string;
  description?: string;
}

export const corsApi = {
  list(): Promise<CorsOriginListResponse> {
    return apiFetch('/admin/platform/cors');
  },

  get(id: string): Promise<CorsOrigin> {
    return apiFetch(`/admin/platform/cors/${id}`);
  },

  getAllowed(): Promise<{ origins: string[] }> {
    return apiFetch('/admin/platform/cors/allowed');
  },

  create(data: CreateCorsOriginRequest): Promise<CorsOrigin> {
    return apiFetch('/admin/platform/cors', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  },

  delete(id: string): Promise<void> {
    return apiFetch(`/admin/platform/cors/${id}`, {
      method: 'DELETE',
    });
  },
};
