/**
 * Permissions Resource
 *
 * Query available permissions.
 */

import type { ResultAsync } from 'neverthrow';
import type { SdkError } from '../errors';
import type { FlowCatalystClient } from '../client';
import * as sdk from '../generated/sdk.gen';
import type {
  GetApiAdminRolesPermissionsResponse,
  GetApiAdminRolesPermissionsByPermissionResponse,
} from '../generated/types.gen';

export type PermissionListResponse = GetApiAdminRolesPermissionsResponse;
export type PermissionDto = GetApiAdminRolesPermissionsByPermissionResponse;

/**
 * Permissions resource for querying available permissions.
 */
export class PermissionsResource {
  constructor(private readonly client: FlowCatalystClient) {}

  /**
   * List all permissions.
   */
  list(): ResultAsync<PermissionListResponse, SdkError> {
    return this.client.request<PermissionListResponse>((httpClient, headers) =>
      sdk.getApiAdminRolesPermissions({
        client: httpClient,
        headers,
      }),
    );
  }

  /**
   * Get a permission by name.
   */
  get(permission: string): ResultAsync<PermissionDto, SdkError> {
    return this.client.request<PermissionDto>((httpClient, headers) =>
      sdk.getApiAdminRolesPermissionsByPermission({
        client: httpClient,
        headers,
        path: { permission },
      }),
    );
  }
}
