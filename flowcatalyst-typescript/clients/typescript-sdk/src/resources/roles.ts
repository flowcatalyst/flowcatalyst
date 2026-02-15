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
  GetApiAdminRolesResponse,
  GetApiAdminRolesByNameResponse,
  PostApiAdminRolesData,
  PutApiAdminRolesByNameData,
  GetApiAdminRolesByApplicationByApplicationIdResponse,
} from '../generated/types.gen';

export type RoleListResponse = GetApiAdminRolesResponse;
export type RoleDto = GetApiAdminRolesByNameResponse;
export type CreateRoleRequest = PostApiAdminRolesData['body'];
export type UpdateRoleRequest = PutApiAdminRolesByNameData['body'];
export type RoleListByApplicationResponse = GetApiAdminRolesByApplicationByApplicationIdResponse;

/**
 * Roles resource for managing role-based access control.
 */
export class RolesResource {
  constructor(private readonly client: FlowCatalystClient) {}

  /**
   * List all roles.
   */
  list(): ResultAsync<RoleListResponse, SdkError> {
    return this.client.request<RoleListResponse>((httpClient, headers) =>
      sdk.getApiAdminRoles({
        client: httpClient,
        headers,
      }),
    );
  }

  /**
   * Get a role by name.
   */
  get(roleName: string): ResultAsync<RoleDto, SdkError> {
    return this.client.request<RoleDto>((httpClient, headers) =>
      sdk.getApiAdminRolesByName({
        client: httpClient,
        headers,
        path: { name: roleName },
      }),
    );
  }

  /**
   * Create a new role.
   */
  create(data: CreateRoleRequest): ResultAsync<RoleDto, SdkError> {
    return this.client.request<RoleDto>((httpClient, headers) =>
      sdk.postApiAdminRoles({
        client: httpClient,
        headers,
        body: data,
      }),
    );
  }

  /**
   * Update a role.
   */
  update(roleName: string, data: UpdateRoleRequest): ResultAsync<RoleDto, SdkError> {
    return this.client.request<RoleDto>((httpClient, headers) =>
      sdk.putApiAdminRolesByName({
        client: httpClient,
        headers,
        path: { name: roleName },
        body: data,
      }),
    );
  }

  /**
   * Delete a role.
   */
  delete(roleName: string): ResultAsync<unknown, SdkError> {
    return this.client.request<unknown>((httpClient, headers) =>
      sdk.deleteApiAdminRolesByName({
        client: httpClient,
        headers,
        path: { name: roleName },
      }),
    );
  }

  /**
   * List roles for an application.
   */
  listForApplication(applicationId: string): ResultAsync<RoleListByApplicationResponse, SdkError> {
    return this.client.request<RoleListByApplicationResponse>((httpClient, headers) =>
      sdk.getApiAdminRolesByApplicationByApplicationId({
        client: httpClient,
        headers,
        path: { applicationId },
      }),
    );
  }
}
