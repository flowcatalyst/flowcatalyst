// Module resolution hooks so the SPA's TypeScript sources can be imported by
// node unchanged: they import siblings as "./x.js" (meaning x.ts), use the
// "@/" alias for frontend/src and the "@flowcatalyst/schema-codegen" package.
import { existsSync } from "node:fs";
import { fileURLToPath, pathToFileURL } from "node:url";
import path from "node:path";

const frontend = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../../../../frontend");

export async function resolve(specifier, context, next) {
	if (specifier === "@flowcatalyst/schema-codegen") {
		return next(pathToFileURL(path.join(frontend, "packages/schema-codegen/src/index.ts")).href, context);
	}
	if (specifier.startsWith("@/")) {
		return next(pathToFileURL(path.join(frontend, "src", specifier.slice(2) + ".ts")).href, context);
	}
	if (specifier.startsWith(".") && specifier.endsWith(".js") && context.parentURL?.endsWith(".ts")) {
		const ts = new URL(specifier.slice(0, -3) + ".ts", context.parentURL);
		if (existsSync(fileURLToPath(ts))) return next(ts.href, context);
	}
	return next(specifier, context);
}
