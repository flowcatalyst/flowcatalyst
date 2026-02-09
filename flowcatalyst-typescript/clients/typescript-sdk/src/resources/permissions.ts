/**
 * Permissions Resource
 *
 * Query available permissions.
 */

import type { ResultAsync } from 'neverthrow';
import type { SdkError } from '../errors';
import type { FlowCatalystClient } from '../client';
import * as sdk from '../generated/sdk.gen';
import type { PermissionDto, PermissionListResponse } from '../generated/types.gen';

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
      sdk.listPermissions({
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
      sdk.getPermission({
        client: httpClient,
        headers,
        path: { permission },
      }),
    );
  }
}
