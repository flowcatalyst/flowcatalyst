import { apiFetch } from "@/api/client";
import type { Client } from "@/api/clients";
import type { Application } from "@/api/applications";
import { useClientOptions } from "@/composables/useClientOptions";
import type { ClientRef } from "@/utils/definition-transfer";

// Best-effort reads for definition export/import. A viewer who can open a
// drawer may not be allowed to read the records it references; those reads
// must fail quietly (no error toast, no permission modal) because the export
// view and the import notice say what was left out.
async function quietGet<T>(path: string): Promise<T | undefined> {
	try {
		return await apiFetch<T>(path, {
			suppressGlobalErrorToast: true,
			suppressAuthErrorEvent: true,
		});
	} catch {
		return undefined;
	}
}

/** The application's code, or undefined when it can't be read. */
export async function readApplicationCode(
	applicationId: string,
): Promise<string | undefined> {
	const app = await quietGet<Application>(`/applications/${applicationId}`);
	return app?.code;
}

/**
 * The clients named by `identifiers` that this form can offer. A client is
 * returned only when the viewer can read it by identifier AND it is among the
 * clients the select offers (active, accessible), so the id is safe to set.
 */
export async function lookupClientsByIdentifier(
	identifiers: string[],
): Promise<ClientRef[]> {
	const unique = [...new Set(identifiers)];
	if (unique.length === 0) return [];
	const { options, ensureLoaded } = useClientOptions();
	try {
		await ensureLoaded();
	} catch {
		return [];
	}
	const found = await Promise.all(
		unique.map((identifier) =>
			quietGet<Client>(`/clients/by-identifier/${encodeURIComponent(identifier)}`),
		),
	);
	const refs: ClientRef[] = [];
	for (const client of found) {
		if (client && options.value.some((o) => o.value === client.id)) {
			refs.push({ id: client.id, identifier: client.identifier });
		}
	}
	return refs;
}
