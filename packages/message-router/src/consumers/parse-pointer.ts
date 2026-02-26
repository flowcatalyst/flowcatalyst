import type { MessagePointer } from "@flowcatalyst/contracts";

/**
 * Parse a raw message body (string or object) into a standardized MessagePointer.
 * Shared by NATS, ActiveMQ, and Embedded consumers that receive unstructured payloads.
 * SQS consumer handles its own parsing due to SQS-specific fields (MessageGroupId attribute, etc.).
 */
export function parseMessagePointer(
	messageId: string,
	raw: unknown,
): MessagePointer {
	let parsed: Record<string, unknown>;

	if (typeof raw === "string") {
		try {
			parsed = JSON.parse(raw) as Record<string, unknown>;
		} catch {
			return {
				messageId,
				poolCode: "DEFAULT",
				messageGroupId: messageId,
				payload: raw,
			};
		}
	} else {
		parsed = (raw ?? {}) as Record<string, unknown>;
	}

	return {
		messageId,
		poolCode: (parsed["poolCode"] as string) || "DEFAULT",
		messageGroupId: (parsed["messageGroupId"] as string) || messageId,
		callbackUrl:
			(parsed["mediationTarget"] as string) ||
			(parsed["callbackUrl"] as string) ||
			undefined,
		authToken: parsed["authToken"] as string | undefined,
		payload: parsed["payload"] ?? parsed,
		highPriority: parsed["highPriority"] === true,
	};
}
