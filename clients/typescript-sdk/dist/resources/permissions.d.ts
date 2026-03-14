/**
 * Permissions Resource
 *
 * Query available permissions.
 */
import type { ResultAsync } from "neverthrow";
import type { SdkError } from "../errors";
import type { FlowCatalystClient } from "../client";
import type { GetApiAdminRolesPermissionsResponse, GetApiAdminRolesPermissionsByPermissionResponse } from "../generated/types.gen";
export type PermissionListResponse = GetApiAdminRolesPermissionsResponse;
export type PermissionDto = GetApiAdminRolesPermissionsByPermissionResponse;
/**
 * Permissions resource for querying available permissions.
 */
export declare class PermissionsResource {
    private readonly client;
    constructor(client: FlowCatalystClient);
    /**
     * List all permissions.
     */
    list(): ResultAsync<PermissionListResponse, SdkError>;
    /**
     * Get a permission by name.
     */
    get(permission: string): ResultAsync<PermissionDto, SdkError>;
}
//# sourceMappingURL=permissions.d.ts.map