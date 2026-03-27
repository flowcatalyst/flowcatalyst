import { apiFetch } from "./client";

export type ConnectionStatus = "ACTIVE" | "PAUSED";

export interface Connection {
	id: string;
	code: string;
	name: string;
	description?: string;
	externalId?: string;
	serviceAccountId: string;
	clientId?: string;
	clientIdentifier?: string;
	status: ConnectionStatus;
	createdAt: string;
	updatedAt: string;
}

export interface ConnectionListResponse {
	connections: Connection[];
	total: number;
}

export interface CreateConnectionRequest {
	code: string;
	name: string;
	description?: string;
	externalId?: string;
	serviceAccountId: string;
	clientId?: string;
}

export interface UpdateConnectionRequest {
	name?: string;
	description?: string;
	externalId?: string;
}

export interface ConnectionFilters {
	clientId?: string;
	status?: ConnectionStatus;
}

export interface StatusResponse {
	message: string;
	connectionId: string;
}

export const connectionsApi = {
	list(filters: ConnectionFilters = {}): Promise<ConnectionListResponse> {
		const params = new URLSearchParams();
		if (filters.clientId) params.set("clientId", filters.clientId);
		if (filters.status) params.set("status", filters.status);

		const query = params.toString();
		return apiFetch(`/admin/connections${query ? `?${query}` : ""}`);
	},

	get(id: string): Promise<Connection> {
		return apiFetch(`/admin/connections/${id}`);
	},

	create(data: CreateConnectionRequest): Promise<Connection> {
		return apiFetch("/admin/connections", {
			method: "POST",
			body: JSON.stringify(data),
		});
	},

	update(id: string, data: UpdateConnectionRequest): Promise<Connection> {
		return apiFetch(`/admin/connections/${id}`, {
			method: "PUT",
			body: JSON.stringify(data),
		});
	},

	delete(id: string): Promise<StatusResponse> {
		return apiFetch(`/admin/connections/${id}`, {
			method: "DELETE",
		});
	},

	pause(id: string): Promise<StatusResponse> {
		return apiFetch(`/admin/connections/${id}/pause`, {
			method: "POST",
		});
	},

	activate(id: string): Promise<StatusResponse> {
		return apiFetch(`/admin/connections/${id}/activate`, {
			method: "POST",
		});
	},
};
