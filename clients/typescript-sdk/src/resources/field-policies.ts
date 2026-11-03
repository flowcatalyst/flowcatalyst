/**
 * Field Policies Resource
 *
 * Per-client field-worker policy (PIN length, lock thresholds, idle and shift
 * limits). Mirrors the Go SDK's `FieldPoliciesResource`.
 */

import type { ResultAsync } from "neverthrow";
import type { SdkError } from "../errors.js";
import type { FlowCatalystClient } from "../client.js";
import * as sdk from "../generated/sdk.gen.js";
import type {
	GetFieldPolicyResponse,
	PutFieldPolicyData,
	PutFieldPolicyResponse,
} from "../generated/types.gen.js";

export type PutFieldPolicyRequest = PutFieldPolicyData["body"];
export type FieldPolicyDto = GetFieldPolicyResponse;

export class FieldPoliciesResource {
	private readonly client: FlowCatalystClient;

	constructor(client: FlowCatalystClient) {
		this.client = client;
	}

	/** The client's effective policy (`isDefault` when none is stored). */
	get(clientId: string): ResultAsync<FieldPolicyDto, SdkError> {
		return this.client.request<FieldPolicyDto>((httpClient, headers) =>
			sdk.getFieldPolicy({ client: httpClient, headers, path: { clientId } }),
		);
	}

	/** Set the policy; omitted members take the platform default. */
	put(
		clientId: string,
		data: PutFieldPolicyRequest,
	): ResultAsync<PutFieldPolicyResponse, SdkError> {
		return this.client.request<PutFieldPolicyResponse>(
			(httpClient, headers) =>
				sdk.putFieldPolicy({
					client: httpClient,
					headers,
					path: { clientId },
					body: data,
				}),
		);
	}
}
