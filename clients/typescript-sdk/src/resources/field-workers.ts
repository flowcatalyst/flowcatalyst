/**
 * Field Workers Resource
 *
 * Admin API for field-worker identities (workers who sign in on enrolled
 * devices with a PIN): ensure/list/get/update/delete, PIN reset, lock and
 * suspend controls, application grants, sign-in history and the optional link
 * to a platform user. Mirrors the Go
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
	ListFieldWorkerBadgesResponse,
	IssueFieldWorkerBadgeData,
	IssueFieldWorkerBadgeResponse,
	RevokeFieldWorkerBadgeResponse,
	IssueFieldWorkerBadgesResponse,
	CreateFieldWorkerSignInCodeResponse,
	LinkFieldWorkerResponse,
	UnlinkFieldWorkerResponse,
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
export type IssueFieldBadgeRequest = NonNullable<
	IssueFieldWorkerBadgeData["body"]
>;
/** A freshly issued badge: `token` is the secret to print, shown once. */
export type FieldBadgeIssued = IssueFieldWorkerBadgeResponse;
export type FieldBadgeListResponse = ListFieldWorkerBadgesResponse;
export type BulkIssueFieldBadgesResult = IssueFieldWorkerBadgesResponse;
/** A single-use code a console admin redeems to sign in as the worker. */
export type FieldSignInCode = CreateFieldWorkerSignInCodeResponse;

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

	/**
	 * Issue a badge (replaces the worker's ACTIVE badge). The response's
	 * `token` is the secret to print as a QR code; it is shown once.
	 */
	issueBadge(
		id: string,
		data: IssueFieldBadgeRequest = {},
	): ResultAsync<FieldBadgeIssued, SdkError> {
		return this.client.request<FieldBadgeIssued>((httpClient, headers) =>
			sdk.issueFieldWorkerBadge({
				client: httpClient,
				headers,
				path: { id },
				body: data,
			}),
		);
	}

	/** The worker's badges, newest first (tokens are never returned). */
	listBadges(id: string): ResultAsync<FieldBadgeListResponse, SdkError> {
		return this.client.request<FieldBadgeListResponse>(
			(httpClient, headers) =>
				sdk.listFieldWorkerBadges({
					client: httpClient,
					headers,
					path: { id },
				}),
		);
	}

	revokeBadge(
		id: string,
		badgeId: string,
	): ResultAsync<RevokeFieldWorkerBadgeResponse, SdkError> {
		return this.client.request<RevokeFieldWorkerBadgeResponse>(
			(httpClient, headers) =>
				sdk.revokeFieldWorkerBadge({
					client: httpClient,
					headers,
					path: { id, badgeId },
				}),
		);
	}

	/**
	 * Issue a badge for each of 1 to 200 workers in one transaction (all or
	 * nothing). Each result carries the token, shown once.
	 */
	bulkIssueBadges(
		workerIds: string[],
	): ResultAsync<BulkIssueFieldBadgesResult, SdkError> {
		return this.client.request<BulkIssueFieldBadgesResult>(
			(httpClient, headers) =>
				sdk.issueFieldWorkerBadges({
					client: httpClient,
					headers,
					body: { workerIds },
				}),
		);
	}

	/**
	 * Mint a single-use code a console admin redeems at the platform's field
	 * sign-in to start an impersonated session as the worker.
	 */
	createSignInCode(
		id: string,
		appCode: string,
	): ResultAsync<FieldSignInCode, SdkError> {
		return this.client.request<FieldSignInCode>((httpClient, headers) =>
			sdk.createFieldWorkerSignInCode({
				client: httpClient,
				headers,
				path: { id },
				body: { appCode },
			}),
		);
	}

	/**
	 * Link the worker to a platform user, for audit correlation only (the
	 * worker's tokens then carry `linked_principal_id`; it grants nothing).
	 * 422 INVALID_LINK when the user cannot be linked, 409 LINK_EXISTS when
	 * another worker of the client already holds the link. Responses carry
	 * `linkedPrincipal`; `list` accepts a `linkedPrincipalId` filter.
	 */
	link(
		id: string,
		principalId: string,
	): ResultAsync<LinkFieldWorkerResponse, SdkError> {
		return this.client.request<LinkFieldWorkerResponse>(
			(httpClient, headers) =>
				sdk.linkFieldWorker({
					client: httpClient,
					headers,
					path: { id },
					body: { principalId },
				}),
		);
	}

	/** Remove the worker's link to a platform user. Idempotent. */
	unlink(id: string): ResultAsync<UnlinkFieldWorkerResponse, SdkError> {
		return this.client.request<UnlinkFieldWorkerResponse>(
			(httpClient, headers) =>
				sdk.unlinkFieldWorker({ client: httpClient, headers, path: { id } }),
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
