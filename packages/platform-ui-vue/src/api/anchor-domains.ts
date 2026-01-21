import { apiFetch } from './client';

export interface AnchorDomain {
  id: string;
  domain: string;
  userCount: number;
  createdAt: string;
}

export interface AnchorDomainListResponse {
  domains: AnchorDomain[];
  total: number;
}

export interface CreateAnchorDomainRequest {
  domain: string;
}

export interface DomainCheckResponse {
  domain: string;
  isAnchorDomain: boolean;
  userCount: number;
}

export interface DeleteAnchorDomainResponse {
  domain: string;
  affectedUsers: number;
  message: string;
}

export const anchorDomainsApi = {
  list(): Promise<AnchorDomainListResponse> {
    return apiFetch('/admin/anchor-domains');
  },

  get(id: string): Promise<AnchorDomain> {
    return apiFetch(`/admin/anchor-domains/${id}`);
  },

  check(domain: string): Promise<DomainCheckResponse> {
    return apiFetch(`/admin/anchor-domains/check/${encodeURIComponent(domain)}`);
  },

  create(data: CreateAnchorDomainRequest): Promise<AnchorDomain> {
    return apiFetch('/admin/anchor-domains', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  },

  delete(id: string): Promise<DeleteAnchorDomainResponse> {
    return apiFetch(`/admin/anchor-domains/${id}`, {
      method: 'DELETE',
    });
  },
};
