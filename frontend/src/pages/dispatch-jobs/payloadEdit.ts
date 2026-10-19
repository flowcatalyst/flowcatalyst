/**
 * Plain logic behind the dispatch-job drawer's "Edit payload" action, kept out
 * of the component so tests/dispatch-jobs-payload-edit.test.ts can pin it. The
 * rules mirror the server's (internal/platform/dispatchjob/operations/
 * edit_payload.go), which is the authority; this only saves a round trip.
 */

/**
 * The statuses a payload may be edited in: PENDING (not yet delivered) and
 * FAILED / its legacy alias ERROR. QUEUED and PROCESSING are being delivered;
 * COMPLETED / CANCELLED / EXPIRED are settled.
 */
export const PAYLOAD_EDITABLE_STATUSES: readonly string[] = ["PENDING", "FAILED", "ERROR"];

/** The server's cap on an edited payload, in UTF-8 bytes. */
export const PAYLOAD_MAX_BYTES = 256 * 1024;

export function canEditPayload(status: string | undefined): boolean {
	return !!status && PAYLOAD_EDITABLE_STATUSES.includes(status);
}

/** application/json or a +json type, ignoring parameters and case. */
export function isJsonContentType(contentType: string | undefined): boolean {
	const mt = (contentType ?? "").split(";")[0]!.trim().toLowerCase();
	return mt === "application/json" || mt.endsWith("+json");
}

export function payloadBytes(text: string): number {
	return new TextEncoder().encode(text).length;
}

/** A media type such as application/json: something, a slash, something. */
export function isValidContentType(contentType: string): boolean {
	const ct = contentType.trim();
	return ct.length > 0 && ct.length <= 100 && ct.includes("/");
}

/** Why the edit cannot be saved, or null when it can. */
export function validatePayloadEdit(text: string, contentType: string): string | null {
	if (!isValidContentType(contentType)) return "Content type must be a media type such as application/json.";
	if (text === "") return "Payload is required.";
	if (payloadBytes(text) > PAYLOAD_MAX_BYTES) return "Payload is larger than the 256 KiB limit.";
	if (isJsonContentType(contentType)) {
		try {
			JSON.parse(text);
		} catch (error) {
			return `Not valid JSON: ${error instanceof Error ? error.message : "parse error"}`;
		}
	}
	return null;
}

/** The text pretty-printed with 2 spaces, or null when it is not JSON. */
export function prettyJson(text: string): string | null {
	try {
		return JSON.stringify(JSON.parse(text), null, 2);
	} catch {
		return null;
	}
}

/** Save only when the text or the content type actually changed. */
export function payloadEditChanged(
	original: { payload?: string | null; contentType?: string },
	text: string,
	contentType: string,
): boolean {
	return text !== (original.payload ?? "") || contentType.trim() !== (original.contentType ?? "");
}
