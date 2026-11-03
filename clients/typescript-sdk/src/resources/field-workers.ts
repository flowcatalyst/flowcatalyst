/**
 * Field Workers Resource
 *
 * Admin API for field-worker identities (workers who sign in on enrolled
 * devices with a PIN): ensure/list/get/update/delete, PIN reset, lock and
 * suspend controls, application grants and sign-in history. Mirrors the Go
 * SDK's `FieldWorkersResource`.
 */

import type { ResultAsync } from "neverthrow";
import type { SdkError } from "../errors.js";
import type { FlowCatalystClient } from "../client.js";
import * as sdk from "../generated/sdk.gen.js";
import type {
	EnsureFieldWorkerData,
	EnsureFieldWorkerResponse,
	ListFieldWorkersData,
	ListFieldWorkersResponse,
	GetFieldWorkerResponse,
	UpdateFieldWorkerData,
	UpdateFieldWorkerResponse,
	DeleteFieldWorkerResponse,
	ResetFieldWorkerPinData,
	ResetFieldWorkerPinResponse,
	UnlockFieldWorkerResponse,
	SuspendFieldWorkerResponse,
	ActivateFieldWorkerResponse,
	GrantFieldWorkerAppResponse,
	RevokeFieldWorkerAppResponse,
	ListFieldWorkerSignInsData,
	ListFieldWorkerSignInsResponse,
} from "../generated/types.gen.js";

export type EnsureFieldWorkerRequest = EnsureFieldWorkerData["body"];
export type UpdateFieldWorkerRequest = UpdateFieldWorkerData["body"];
export type ResetFieldWorkerPinRequest = ResetFieldWorkerPinData["body"];
export type FieldWorkerFilters = NonNullable<ListFieldWorkersData["query"]>;
export type FieldWorkerSignInPage = NonNullable<
	ListFieldWorkerSignInsData["query"]
>;
export type FieldWorkerDto = EnsureFieldWorkerResponse;
export type FieldWorkerListResponse = ListFieldWorkersResponse;
export type FieldWorkerSignInListResponse = ListFieldWorkerSignInsResponse;
export type ResetFieldWorkerPinResult = ResetFieldWorkerPinResponse;

export class FieldWorkersResource {
	private readonly client: FlowCatalystClient;

	constructor(client: FlowCatalystClient) {
		this.client = client;
	}

	/**
	 * Create the worker for (clientId, workerCode) if absent and grant the
	 * listed applications; the response's `created` says which happened.
	 */
	ensure(
		data: EnsureFieldWorkerRequest,
	): ResultAsync<FieldWorkerDto, SdkError> {
		return this.client.request<FieldWorkerDto>((httpClient, headers) =>
			sdk.ensureFieldWorker({ client: httpClient, headers, body: data }),
		);
	}

	/** List one client's workers, with optional filters. */
	list(
		clientId: string,
		filters?: Omit<FieldWorkerFilters, "clientId">,
	): ResultAsync<FieldWorkerListResponse, SdkError> {
		return this.client.request<FieldWorkerListResponse>(
			(httpClient, headers) =>
				sdk.listFieldWorkers({
					client: httpClient,
					headers,
					query: { ...filters, clientId },
				}),
		);
	}

	get(id: string): ResultAsync<GetFieldWorkerResponse, SdkError> {
		return this.client.request<GetFieldWorkerResponse>(
			(httpClient, headers) =>
				sdk.getFieldWorker({ client: httpClient, headers, path: { id } }),
		);
	}

	update(
		id: string,
		data: UpdateFieldWorkerRequest,
	): ResultAsync<UpdateFieldWorkerResponse, SdkError> {
		return this.client.request<UpdateFieldWorkerResponse>(
			(httpClient, headers) =>
				sdk.updateFieldWorker({
					client: httpClient,
					headers,
					path: { id },
					body: data,
				}),
		);
	}

	/** Revokes the worker's sessions first. */
	delete(id: string): ResultAsync<DeleteFieldWorkerResponse, SdkError> {
		return this.client.request<DeleteFieldWorkerResponse>(
			(httpClient, headers) =>
				sdk.deleteFieldWorker({ client: httpClient, headers, path: { id } }),
		);
	}

	/**
	 * Omit `pin` for a random temporary PIN, returned once in
	 * `temporaryPin`.
	 */
	resetPin(
		id: string,
		data: ResetFieldWorkerPinRequest = {},
	): ResultAsync<ResetFieldWorkerPinResult, SdkError> {
		return this.client.request<ResetFieldWorkerPinResult>(
			(httpClient, headers) =>
				sdk.resetFieldWorkerPin({
					client: httpClient,
					headers,
					path: { id },
					body: data,
				}),
		);
	}

	/** Clears soft and hard locks. */
	unlock(id: string): ResultAsync<UnlockFieldWorkerResponse, SdkError> {
		return this.client.request<UnlockFieldWorkerResponse>(
			(httpClient, headers) =>
				sdk.unlockFieldWorker({ client: httpClient, headers, path: { id } }),
		);
	}

	/** Revokes the worker's sessions. */
	suspend(id: string): ResultAsync<SuspendFieldWorkerResponse, SdkError> {
		return this.client.request<SuspendFieldWorkerResponse>(
			(httpClient, headers) =>
				sdk.suspendFieldWorker({ client: httpClient, headers, path: { id } }),
		);
	}

	activate(id: string): ResultAsync<ActivateFieldWorkerResponse, SdkError> {
		return this.client.request<ActivateFieldWorkerResponse>(
			(httpClient, headers) =>
				sdk.activateFieldWorker({ client: httpClient, headers, path: { id } }),
		);
	}

	grantApp(
		id: string,
		appCode: string,
	): ResultAsync<GrantFieldWorkerAppResponse, SdkError> {
		return this.client.request<GrantFieldWorkerAppResponse>(
			(httpClient, headers) =>
				sdk.grantFieldWorkerApp({
					client: httpClient,
					headers,
					path: { id },
					body: { appCode },
				}),
		);
	}

	/** Also revokes that application's sessions. */
	revokeApp(
		id: string,
		appCode: string,
	): ResultAsync<RevokeFieldWorkerAppResponse, SdkError> {
		return this.client.request<RevokeFieldWorkerAppResponse>(
			(httpClient, headers) =>
				sdk.revokeFieldWorkerApp({
					client: httpClient,
					headers,
					path: { id, appCode },
				}),
		);
	}

	/** Sign-in attempts, newest first. */
	signIns(
		id: string,
		page?: FieldWorkerSignInPage,
	): ResultAsync<FieldWorkerSignInListResponse, SdkError> {
		return this.client.request<FieldWorkerSignInListResponse>(
			(httpClient, headers) =>
				sdk.listFieldWorkerSignIns({
					client: httpClient,
					headers,
					path: { id },
					...(page ? { query: page } : {}),
				}),
		);
	}
}
