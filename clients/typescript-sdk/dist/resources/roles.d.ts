/**
 * Roles Resource
 *
 * Manage roles and permissions.
 */
import type { ResultAsync } from "neverthrow";
import type { SdkError } from "../errors";
import type { FlowCatalystClient } from "../client";
import type { GetApiAdminRolesResponse, GetApiAdminRolesByNameResponse, PostApiAdminRolesData, PutApiAdminRolesByNameData, GetApiAdminRolesByApplicationByApplicationIdResponse } from "../generated/types.gen";
export type RoleListResponse = GetApiAdminRolesResponse;
export type RoleDto = GetApiAdminRolesByNameResponse;
export type CreateRoleRequest = PostApiAdminRolesData["body"];
export type UpdateRoleRequest = PutApiAdminRolesByNameData["body"];
export type RoleListByApplicationResponse = GetApiAdminRolesByApplicationByApplicationIdResponse;
/**
 * Roles resource for managing role-based access control.
 */
export declare class RolesResource {
    private readonly client;
    constructor(client: FlowCatalystClient);
    /**
     * List all roles.
     */
    list(): ResultAsync<RoleListResponse, SdkError>;
    /**
     * Get a role by name.
     */
    get(roleName: string): ResultAsync<RoleDto, SdkError>;
    /**
     * Create a new role.
     */
    create(data: CreateRoleRequest): ResultAsync<RoleDto, SdkError>;
    /**
     * Update a role.
     */
    update(roleName: string, data: UpdateRoleRequest): ResultAsync<RoleDto, SdkError>;
    /**
     * Delete a role.
     */
    delete(roleName: string): ResultAsync<unknown, SdkError>;
    /**
     * List roles for an application.
     */
    listForApplication(applicationId: string): ResultAsync<RoleListByApplicationResponse, SdkError>;
}
//# sourceMappingURL=roles.d.ts.map