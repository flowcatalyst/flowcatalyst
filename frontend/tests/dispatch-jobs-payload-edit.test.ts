import { describe, expect, it } from "vitest";
import {
	canEditPayload,
	isJsonContentType,
	payloadBytes,
	payloadEditChanged,
	prettyJson,
	validatePayloadEdit,
} from "@/pages/dispatch-jobs/payloadEdit";

describe("canEditPayload", () => {
	it("allows PENDING, FAILED and the legacy ERROR", () => {
		for (const s of ["PENDING", "FAILED", "ERROR"]) expect(canEditPayload(s)).toBe(true);
	});
	it("refuses in-flight and settled jobs", () => {
		for (const s of ["QUEUED", "PROCESSING", "COMPLETED", "CANCELLED", "EXPIRED", "", undefined]) {
			expect(canEditPayload(s)).toBe(false);
		}
	});
});

describe("isJsonContentType", () => {
	it("recognises json types", () => {
		expect(isJsonContentType("application/json")).toBe(true);
		expect(isJsonContentType("Application/JSON; charset=utf-8")).toBe(true);
		expect(isJsonContentType("application/vnd.api+json")).toBe(true);
		expect(isJsonContentType("application/xml")).toBe(false);
		expect(isJsonContentType(undefined)).toBe(false);
	});
});

describe("validatePayloadEdit", () => {
	it("accepts valid JSON and non-JSON for other types", () => {
		expect(validatePayloadEdit('{"a":1}', "application/json")).toBeNull();
		expect(validatePayloadEdit("<a/>", "application/xml")).toBeNull();
	});
	it("refuses invalid JSON for a JSON type only", () => {
		expect(validatePayloadEdit("{nope", "application/json")).toMatch(/Not valid JSON/);
		expect(validatePayloadEdit("{nope", "text/plain")).toBeNull();
	});
	it("refuses empty and malformed content type", () => {
		expect(validatePayloadEdit("", "application/json")).toBe("Payload is required.");
		expect(validatePayloadEdit("{}", "json")).toMatch(/media type/);
		expect(validatePayloadEdit("{}", "  ")).toMatch(/media type/);
	});
	it("leaves the size to the server, whose limit is the deployment's", () => {
		// Over the 256 KiB the SPA used to refuse by itself: the server takes up
		// to its largest payload (1 MiB or 5 MB) and refuses more with
		// PAYLOAD_TOO_LARGE, shown as any refusal.
		expect(validatePayloadEdit("a".repeat(256 * 1024 + 1), "text/plain")).toBeNull();
		expect(validatePayloadEdit(`"${"a".repeat(2 * 1024 * 1024)}"`, "application/json")).toBeNull();
	});
	it("counts bytes, not characters", () => {
		expect(payloadBytes("é".repeat(10))).toBe(20);
		expect(payloadBytes("abc")).toBe(3);
	});
});

describe("prettyJson", () => {
	it("formats JSON and returns null otherwise", () => {
		expect(prettyJson('{"a":[1,2]}')).toBe('{\n  "a": [\n    1,\n    2\n  ]\n}');
		expect(prettyJson("<a/>")).toBeNull();
	});
});

describe("payloadEditChanged", () => {
	const orig = { payload: '{"a":1}', contentType: "application/json" };
	it("detects text and content type changes", () => {
		expect(payloadEditChanged(orig, '{"a":1}', "application/json")).toBe(false);
		expect(payloadEditChanged(orig, '{"a":2}', "application/json")).toBe(true);
		expect(payloadEditChanged(orig, '{"a":1}', "application/xml")).toBe(true);
	});
});
