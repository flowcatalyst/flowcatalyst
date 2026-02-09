/**
 * Principals Resource
 *
 * Manage users and service accounts.
 */

import type { ResultAsync } from 'neverthrow';
import type { SdkError } from '../errors';
import type { FlowCatalystClient } from '../client';
import * as sdk from '../generated/sdk.gen';
import type {
  PrincipalDto,
  PrincipalListResponse,
  CreateUserRequest,
  UpdatePrincipalRequest,
  RoleListResponse,
  ClientAccessListResponse,
  PrincipalType,
} from '../generated/types.gen';

export interface PrincipalFilters {
  clientId?: string;
  type?: PrincipalType;
  active?: boolean;
  email?: string;
}

/**
 * Principals resource for managing users and service accounts.
 */
export class PrincipalsResource {
  constructor(private readonly client: FlowCatalystClient) {}

  /**
   * List all principals with optional filters.
   */
  list(filters?: PrincipalFilters): ResultAsync<PrincipalListResponse, SdkError> {
    return this.client.request<PrincipalListResponse>((httpClient, headers) =>
      sdk.listPrincipals({
        client: httpClient,
        headers,
        query: filters,
      }),
    );
  }

  /**
   * Get a principal by ID.
   */
  get(id: string): ResultAsync<PrincipalDto, SdkError> {
    return this.client.request<PrincipalDto>((httpClient, headers) =>
      sdk.getPrincipal({
        client: httpClient,
        headers,
        path: { id },
      }),
    );
  }

  /**
   * Find a user by email.
   */
  findByEmail(email: string): ResultAsync<PrincipalListResponse, SdkError> {
    return this.list({ email });
  }

  /**
   * Create a new user principal.
   */
  createUser(data: CreateUserRequest): ResultAsync<PrincipalDto, SdkError> {
    return this.client.request<PrincipalDto>((httpClient, headers) =>
      sdk.createUser({
        client: httpClient,
        headers,
        body: data,
      }),
    );
  }

  /**
   * Update a principal.
   */
  update(id: string, data: UpdatePrincipalRequest): ResultAsync<PrincipalDto, SdkError> {
    return this.client.request<PrincipalDto>((httpClient, headers) =>
      sdk.updatePrincipal({
        client: httpClient,
        headers,
        path: { id },
        body: data,
      }),
    );
  }

  /**
   * Activate a principal.
   */
  activate(id: string): ResultAsync<PrincipalDto, SdkError> {
    return this.client.request<PrincipalDto>((httpClient, headers) =>
      sdk.activatePrincipal({
        client: httpClient,
        headers,
        path: { id },
      }),
    );
  }

  /**
   * Deactivate a principal.
   */
  deactivate(id: string): ResultAsync<PrincipalDto, SdkError> {
    return this.client.request<PrincipalDto>((httpClient, headers) =>
      sdk.deactivatePrincipal({
        client: httpClient,
        headers,
        path: { id },
      }),
    );
  }

  /**
   * Get roles assigned to a principal.
   */
  getRoles(id: string): ResultAsync<RoleListResponse, SdkError> {
    return this.client.request<RoleListResponse>((httpClient, headers) =>
      sdk.getPrincipalRoles({
        client: httpClient,
        headers,
        path: { id },
      }),
    );
  }

  /**
   * Assign a single role to a principal.
   */
  assignRole(id: string, roleName: string): ResultAsync<unknown, SdkError> {
    return this.client.request<unknown>((httpClient, headers) =>
      sdk.assignPrincipalRole({
        client: httpClient,
        headers,
        path: { id },
        body: { roleName },
      }),
    );
  }

  /**
   * Remove a role from a principal.
   */
  removeRole(id: string, roleName: string): ResultAsync<unknown, SdkError> {
    return this.client.request<unknown>((httpClient, headers) =>
      sdk.removePrincipalRole({
        client: httpClient,
        headers,
        path: { id, roleName },
      }),
    );
  }

  /**
   * Assign roles to a principal (declarative - replaces all roles).
   */
  assignRoles(id: string, roles: string[]): ResultAsync<unknown, SdkError> {
    return this.client.request<unknown>((httpClient, headers) =>
      sdk.assignPrincipalRoles({
        client: httpClient,
        headers,
        path: { id },
        body: { roles },
      }),
    );
  }

  /**
   * Get client access grants for a principal.
   */
  getClientAccessGrants(id: string): ResultAsync<ClientAccessListResponse, SdkError> {
    return this.client.request<ClientAccessListResponse>((httpClient, headers) =>
      sdk.getPrincipalClientAccess({
        client: httpClient,
        headers,
        path: { id },
      }),
    );
  }

  /**
   * Grant client access to a principal.
   */
  grantClientAccess(id: string, clientId: string): ResultAsync<unknown, SdkError> {
    return this.client.request<unknown>((httpClient, headers) =>
      sdk.grantPrincipalClientAccess({
        client: httpClient,
        headers,
        path: { id },
        body: { clientId },
      }),
    );
  }

  /**
   * Revoke client access from a principal.
   */
  revokeClientAccess(id: string, clientId: string): ResultAsync<unknown, SdkError> {
    return this.client.request<unknown>((httpClient, headers) =>
      sdk.revokePrincipalClientAccess({
        client: httpClient,
        headers,
        path: { id, clientId },
      }),
    );
  }
}
