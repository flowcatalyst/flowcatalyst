/**
 * Schema Export
 *
 * Builds a ZIP file of all CURRENT JSON Schemas from the visible event types
 * and triggers a browser download.
 *
 * File structure inside the ZIP:
 *   {application}/{subdomain}/{aggregate}/{event}.json
 */

import { zipSync, strToU8 } from "fflate";
import type { EventType } from "@/api/event-types";

export function exportSchemasAsZip(eventTypes: EventType[]): void {
	const files: Record<string, Uint8Array> = {};
	let count = 0;

	for (const et of eventTypes) {
		const current = et.specVersions.find((sv) => sv.status === "CURRENT");
		if (!current?.schema) continue;

		const path = `${et.application}/${et.subdomain}/${et.aggregate}/${et.event}.json`;
		const content = formatSchema(current.schema);
		files[path] = strToU8(content);
		count++;
	}

	if (count === 0) return;

	const zip = zipSync(files);
	const blob = new Blob([zip], { type: "application/zip" });
	downloadBlob(blob, "event-schemas.zip");
}

function formatSchema(schema: string): string {
	try {
		return JSON.stringify(JSON.parse(schema), null, 2);
	} catch {
		return schema;
	}
}

function downloadBlob(blob: Blob, filename: string): void {
	const url = URL.createObjectURL(blob);
	const a = document.createElement("a");
	a.href = url;
	a.download = filename;
	a.click();
	URL.revokeObjectURL(url);
}
