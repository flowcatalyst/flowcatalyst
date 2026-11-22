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
	SetFieldWorkerPhotoResponse,
	RemoveFieldWorkerPhotoResponse,
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

/** A worker's photo as `getPhoto` reads it. */
export interface FieldWorkerPhotoImage {
	/** The JPEG; `undefined` when `notModified`. */
	bytes?: Uint8Array;
	contentType?: string;
	/** The validator (`"<version>-<size>"`, quotes included): pass it as `ifNoneMatch` next time. */
	etag?: string;
	/** True when the platform answered 304 to `ifNoneMatch`. */
	notModified: boolean;
}

/** Which rendition: `thumb` (128 px longest side) or `full` (512 px). */
export type FieldWorkerPhotoSize = "thumb" | "full";

export class FieldWorkersResource {
	private readonly client: FlowCatalystClient;

	constructor(client: FlowCatalystClient) {
		this.client = client;
	}

	/**
	 * Create the worker for (clientId, workerCode) if absent and grant the
	 * listed applications; the response's `created` says which happened.
	 * Without a `workerCode` the platform generates one (three letters, then
	 * the client policy's `workerCodeDigits` digits, e.g. `KTM482`) and the
	 * response carries it.
	 *
	 * A request without a code that carries an `idempotencyKey` is repeatable
	 * (the platform answers a repeat with the worker the first one made), so it
	 * is retried after a lost answer. Without a key it is not sent again, and a
	 * lost answer ends with `outcome_unknown`: a second send could make a second
	 * worker. A request with a `workerCode` is sent as before.
	 */
	ensure(
		data: EnsureFieldWorkerRequest,
	): ResultAsync<FieldWorkerDto, SdkError> {
		const repeatable = !data.workerCode?.trim() && !!data.idempotencyKey?.trim();
		return this.client.request<FieldWorkerDto>(
			(httpClient, headers) =>
				sdk.ensureFieldWorker({ client: httpClient, headers, body: data }),
			{ repeatable },
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

	/**
	 * Set or replace the worker's photo. `image` is the picture itself (JPEG,
	 * PNG or WebP; the platform decides the type from the bytes and
	 * `contentType` may only agree: omit it for `application/octet-stream`). The
	 * platform scales it to 512 px (full) and 128 px (thumb) and keeps nothing
	 * else of the upload, metadata included. Returns the refreshed worker.
	 *
	 * Refusals are errors; the platform's code is in the error's body (http
	 * errors) or `code` (conflict): `PHOTO_EMPTY`, `PHOTO_TOO_LARGE` (413),
	 * `PHOTO_UNSUPPORTED_TYPE` (415), `PHOTO_TYPE_MISMATCH`,
	 * `PHOTO_DIMENSIONS_TOO_LARGE`, `PHOTO_UNREADABLE`, `PHOTO_BUSY` (503),
	 * `PHOTOS_DISABLED_FOR_CLIENT` (409). `PHOTOS_DISABLED` (404) is a `not_found`.
	 */
	setPhoto(
		id: string,
		image: Blob | Uint8Array | ArrayBuffer,
		contentType = "application/octet-stream",
	): ResultAsync<SetFieldWorkerPhotoResponse, SdkError> {
		const body =
			image instanceof Blob ? image : new Blob([image as BlobPart], { type: contentType });
		return this.client.request<SetFieldWorkerPhotoResponse>(
			(httpClient, headers) =>
				sdk.setFieldWorkerPhoto({
					client: httpClient,
					headers: { ...headers, "Content-Type": contentType },
					path: { id },
					body,
				}),
		);
	}

	/**
	 * Read the worker's photo as bytes (`size` defaults to `thumb`). Pass the
	 * `etag` of an earlier read as `ifNoneMatch`: while the photo is unchanged the
	 * answer is `{ notModified: true }` with no bytes. A worker with no photo (or a
	 * client with photos off) is a `not_found` error (the platform's code is
	 * `PHOTO_NOT_FOUND`).
	 */
	getPhoto(
		id: string,
		options: { size?: FieldWorkerPhotoSize; ifNoneMatch?: string } = {},
	): ResultAsync<FieldWorkerPhotoImage, SdkError> {
		return this.client.request<FieldWorkerPhotoImage>(
			async (httpClient, headers) => {
				const r = await sdk.getFieldWorkerPhoto({
					client: httpClient,
					headers: {
						...headers,
						...(options.ifNoneMatch ? { "If-None-Match": options.ifNoneMatch } : {}),
					},
					path: { id },
					...(options.size ? { query: { size: options.size } } : {}),
					parseAs: "arrayBuffer",
				});
				// The generated client treats a 304 as an error and reads an image as a
				// Blob: answer both as this method documents.
				if (r.response.status === 304) {
					return {
						data: { notModified: true, etag: r.response.headers.get("etag") ?? options.ifNoneMatch },
						response: new Response(null, { status: 200 }),
					};
				}
				if (r.response.ok && r.data !== undefined) {
					return {
						data: {
							bytes: new Uint8Array(r.data as unknown as ArrayBuffer),
							contentType: r.response.headers.get("content-type") ?? undefined,
							etag: r.response.headers.get("etag") ?? undefined,
							notModified: false,
						},
						response: r.response,
					};
				}
				return r;
			},
		);
	}

	/** Remove the worker's photo. Not an error when there is none. Returns the refreshed worker. */
	removePhoto(id: string): ResultAsync<RemoveFieldWorkerPhotoResponse, SdkError> {
		return this.client.request<RemoveFieldWorkerPhotoResponse>(
			(httpClient, headers) =>
				sdk.removeFieldWorkerPhoto({ client: httpClient, headers, path: { id } }),
		);
	}
}
