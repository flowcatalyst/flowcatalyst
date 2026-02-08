/**
 * Roles Resource
 *
 * Manage roles and permissions.
 */

import type { ResultAsync } from 'neverthrow';
import type { SdkError } from '../errors';
import type { FlowCatalystClient } from '../client';
import * as sdk from '../generated/sdk.gen';
import type {
  RoleDto,
  RoleListResponse2,
  CreateRoleRequest,
  UpdateRoleRequest,
  SyncRolesRequest,
  SyncResponse2,
  RoleListResponse1,
} from '../generated/types.gen';

/**
 * Roles resource for managing role-based access control.
 */
export class RolesResource {
  constructor(private readonly client: FlowCatalystClient) {}

  /**
   * List all roles.
   */
  list(): ResultAsync<RoleListResponse2, SdkError> {
    return this.client.request<RoleListResponse2>((httpClient, headers) =>
      sdk.listRoles({
        client: httpClient,
        headers,
      })
    );
  }

  /**
   * Get a role by name.
   */
  get(roleName: string): ResultAsync<RoleDto, SdkError> {
    return this.client.request<RoleDto>((httpClient, headers) =>
      sdk.getRole({
        client: httpClient,
        headers,
        path: { roleName },
      })
    );
  }

  /**
   * Create a new role.
   */
  create(data: CreateRoleRequest): ResultAsync<RoleDto, SdkError> {
    return this.client.request<RoleDto>((httpClient, headers) =>
      sdk.createRole({
        client: httpClient,
        headers,
        body: data,
      })
    );
  }

  /**
   * Update a role.
   */
  update(roleName: string, data: UpdateRoleRequest): ResultAsync<RoleDto, SdkError> {
    return this.client.request<RoleDto>((httpClient, headers) =>
      sdk.updateRole({
        client: httpClient,
        headers,
        path: { roleName },
        body: data,
      })
    );
  }

  /**
   * Delete a role.
   */
  delete(roleName: string): ResultAsync<unknown, SdkError> {
    return this.client.request<unknown>((httpClient, headers) =>
      sdk.deleteRole({
        client: httpClient,
        headers,
        path: { roleName },
      })
    );
  }

  /**
   * List roles for an application.
   */
  listForApplication(appCode: string): ResultAsync<RoleListResponse1, SdkError> {
    return this.client.request<RoleListResponse1>((httpClient, headers) =>
      sdk.listApplicationRoles({
        client: httpClient,
        headers,
        path: { appCode },
      })
    );
  }

  /**
   * Sync roles for an application.
   */
  sync(
    appCode: string,
    roles: SyncRolesRequest['roles'],
    removeUnlisted = false
  ): ResultAsync<SyncResponse2, SdkError> {
    return this.client.request<SyncResponse2>((httpClient, headers) =>
      sdk.syncApplicationRoles({
        client: httpClient,
        headers,
        path: { appCode },
        query: { removeUnlisted },
        body: { roles },
      })
    );
  }
}
