/**
 * Field Devices Resource
 *
 * Admin API for enrolled field devices and their single-use enrolment codes.
 * Mirrors the Go SDK's `FieldDevicesResource`.
 */

import type { ResultAsync } from "neverthrow";
import type { SdkError } from "../errors.js";
import type { FlowCatalystClient } from "../client.js";
import * as sdk from "../generated/sdk.gen.js";
import type {
	CreateFieldEnrolmentCodeData,
	CreateFieldEnrolmentCodeResponse,
	ListFieldDevicesData,
	ListFieldDevicesResponse,
	GetFieldDeviceResponse,
	SuspendFieldDeviceResponse,
	ActivateFieldDeviceResponse,
	RevokeFieldDeviceResponse,
} from "../generated/types.gen.js";

export type CreateEnrolmentCodeRequest = CreateFieldEnrolmentCodeData["body"];
export type FieldDeviceFilters = NonNullable<ListFieldDevicesData["query"]>;
export type FieldEnrolmentCode = CreateFieldEnrolmentCodeResponse;
export type FieldDeviceDto = GetFieldDeviceResponse;
export type FieldDeviceListResponse = ListFieldDevicesResponse;

export class FieldDevicesResource {
	private readonly client: FlowCatalystClient;

	constructor(client: FlowCatalystClient) {
		this.client = client;
	}

	/** Mint a single-use enrolment code (render `qrPayload` as a QR code). */
	createEnrolmentCode(
		data: CreateEnrolmentCodeRequest,
	): ResultAsync<FieldEnrolmentCode, SdkError> {
		return this.client.request<FieldEnrolmentCode>((httpClient, headers) =>
			sdk.createFieldEnrolmentCode({
				client: httpClient,
				headers,
				body: data,
			}),
		);
	}

	/** List one client's devices, with optional filters. */
	list(
		clientId: string,
		filters?: Omit<FieldDeviceFilters, "clientId">,
	): ResultAsync<FieldDeviceListResponse, SdkError> {
		return this.client.request<FieldDeviceListResponse>(
			(httpClient, headers) =>
				sdk.listFieldDevices({
					client: httpClient,
					headers,
					query: { ...filters, clientId },
				}),
		);
	}

	get(id: string): ResultAsync<FieldDeviceDto, SdkError> {
		return this.client.request<FieldDeviceDto>((httpClient, headers) =>
			sdk.getFieldDevice({ client: httpClient, headers, path: { id } }),
		);
	}

	/** Revokes the device's sessions. */
	suspend(id: string): ResultAsync<SuspendFieldDeviceResponse, SdkError> {
		return this.client.request<SuspendFieldDeviceResponse>(
			(httpClient, headers) =>
				sdk.suspendFieldDevice({ client: httpClient, headers, path: { id } }),
		);
	}

	activate(id: string): ResultAsync<ActivateFieldDeviceResponse, SdkError> {
		return this.client.request<ActivateFieldDeviceResponse>(
			(httpClient, headers) =>
				sdk.activateFieldDevice({ client: httpClient, headers, path: { id } }),
		);
	}

	/** Permanent; revokes the device's sessions. */
	revoke(id: string): ResultAsync<RevokeFieldDeviceResponse, SdkError> {
		return this.client.request<RevokeFieldDeviceResponse>(
			(httpClient, headers) =>
				sdk.revokeFieldDevice({ client: httpClient, headers, path: { id } }),
		);
	}
}
