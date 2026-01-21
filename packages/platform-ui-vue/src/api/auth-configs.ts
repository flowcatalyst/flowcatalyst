import { apiFetch } from './client';

export type AuthProvider = 'INTERNAL' | 'OIDC';

export type AuthConfigType = 'ANCHOR' | 'PARTNER' | 'CLIENT';

export interface AuthConfig {
  id: string;
  emailDomain: string;
  configType: AuthConfigType;
  primaryClientId: string | null;
  additionalClientIds: string[];
  grantedClientIds: string[];
  /** @deprecated Use primaryClientId instead */
  clientId: string | null;
  authProvider: AuthProvider;
  oidcIssuerUrl: string | null;
  oidcClientId: string | null;
  hasClientSecret: boolean;
  oidcMultiTenant: boolean;
  oidcIssuerPattern: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface AuthConfigListResponse {
  configs: AuthConfig[];
  total: number;
}

export interface CreateInternalConfigRequest {
  emailDomain: string;
  configType: AuthConfigType;
  primaryClientId?: string | null;
  /** @deprecated Use configType and primaryClientId instead */
  clientId?: string | null;
}

export interface CreateOidcConfigRequest {
  emailDomain: string;
  configType: AuthConfigType;
  primaryClientId?: string | null;
  /** @deprecated Use configType and primaryClientId instead */
  clientId?: string | null;
  oidcIssuerUrl: string;
  oidcClientId: string;
  oidcClientSecretRef?: string;
  oidcMultiTenant?: boolean;
  oidcIssuerPattern?: string;
}

export interface UpdateOidcConfigRequest {
  oidcIssuerUrl: string;
  oidcClientId: string;
  oidcClientSecretRef?: string;
  oidcMultiTenant?: boolean;
  oidcIssuerPattern?: string;
}

export interface UpdateClientBindingRequest {
  clientId: string | null;
}

export interface UpdateConfigTypeRequest {
  configType: AuthConfigType;
  primaryClientId?: string | null;
}

export interface UpdateAdditionalClientsRequest {
  additionalClientIds: string[];
}

export interface UpdateGrantedClientsRequest {
  grantedClientIds: string[];
}

export interface ValidateSecretRequest {
  secretRef: string;
}

export interface SecretValidationResponse {
  valid: boolean;
  message: string;
}

export const authConfigsApi = {
  list(clientId?: string): Promise<AuthConfigListResponse> {
    const params = clientId ? `?clientId=${clientId}` : '';
    return apiFetch(`/admin/auth-configs${params}`);
  },

  get(id: string): Promise<AuthConfig> {
    return apiFetch(`/admin/auth-configs/${id}`);
  },

  getByDomain(domain: string): Promise<AuthConfig> {
    return apiFetch(`/admin/auth-configs/by-domain/${encodeURIComponent(domain)}`);
  },

  createInternal(data: CreateInternalConfigRequest): Promise<AuthConfig> {
    return apiFetch('/admin/auth-configs/internal', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  },

  createOidc(data: CreateOidcConfigRequest): Promise<AuthConfig> {
    return apiFetch('/admin/auth-configs/oidc', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  },

  updateOidc(id: string, data: UpdateOidcConfigRequest): Promise<AuthConfig> {
    return apiFetch(`/admin/auth-configs/${id}/oidc`, {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  },

  updateClientBinding(id: string, clientId: string | null): Promise<AuthConfig> {
    return apiFetch(`/admin/auth-configs/${id}/client-binding`, {
      method: 'PUT',
      body: JSON.stringify({ clientId }),
    });
  },

  updateConfigType(id: string, data: UpdateConfigTypeRequest): Promise<AuthConfig> {
    return apiFetch(`/admin/auth-configs/${id}/config-type`, {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  },

  updateAdditionalClients(id: string, additionalClientIds: string[]): Promise<AuthConfig> {
    return apiFetch(`/admin/auth-configs/${id}/additional-clients`, {
      method: 'PUT',
      body: JSON.stringify({ additionalClientIds }),
    });
  },

  updateGrantedClients(id: string, grantedClientIds: string[]): Promise<AuthConfig> {
    return apiFetch(`/admin/auth-configs/${id}/granted-clients`, {
      method: 'PUT',
      body: JSON.stringify({ grantedClientIds }),
    });
  },

  delete(id: string): Promise<void> {
    return apiFetch(`/admin/auth-configs/${id}`, {
      method: 'DELETE',
    });
  },

  validateSecret(secretRef: string): Promise<SecretValidationResponse> {
    return apiFetch('/admin/auth-configs/validate-secret', {
      method: 'POST',
      body: JSON.stringify({ secretRef }),
    });
  },
};
