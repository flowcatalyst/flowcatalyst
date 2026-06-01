/**
 * Permissions Resource
 *
 * Query available permissions.
 */

import type { ResultAsync } from "neverthrow";
import type { SdkError } from "../errors.js";
import type { FlowCatalystClient } from "../client.js";
import * as sdk from "../generated/sdk.gen.js";
import type {
	GetApiRolesPermissionsResponse,
	GetApiRolesPermissionsByPermissionResponse,
} from "../generated/types.gen.js";

export type PermissionListResponse = GetApiRolesPermissionsResponse;
export type PermissionDto = GetApiRolesPermissionsByPermissionResponse;

/**
 * Permissions resource for querying available permissions.
 */
export class PermissionsResource {
	private readonly client: FlowCatalystClient;

	constructor(client: FlowCatalystClient) {
		this.client = client;
	}

	/**
	 * List all permissions.
	 */
	list(): ResultAsync<PermissionListResponse, SdkError> {
		return this.client.request<PermissionListResponse>((httpClient, headers) =>
			sdk.getApiRolesPermissions({
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
			sdk.getApiRolesPermissionsByPermission({
				client: httpClient,
				headers,
				path: { permission },
			}),
		);
	}
}
