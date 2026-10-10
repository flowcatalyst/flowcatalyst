// Regenerates testdata/golden from the ORIGINAL TypeScript sources.
//
//   node internal/schemacodegen/testdata/gen/gen.mjs
//
// (or `go generate ./internal/schemacodegen/`). Needs node >= 22.18 (type
// stripping) and frontend/node_modules (for fflate).
import { register } from "node:module";
import { readFileSync, writeFileSync, readdirSync, mkdirSync, rmSync } from "node:fs";
import { fileURLToPath, pathToFileURL } from "node:url";
import path from "node:path";

register("./hooks.mjs", import.meta.url);

const here = path.dirname(fileURLToPath(import.meta.url));
const testdata = path.resolve(here, "..");
const frontend = path.resolve(testdata, "../../../frontend");
const src = (rel) => pathToFileURL(path.join(frontend, rel)).href;

const codegen = await import(src("packages/schema-codegen/src/index.ts"));
const hl = await import(src("src/utils/schema-highlight.ts"));
const exporter = await import(src("src/utils/schema-export.ts"));
const { unzipSync, strFromU8 } = await import(src("node_modules/fflate/esm/index.mjs"));

// ---- SchemaViewerDialog.vue computed properties, transcribed -------------
// (the .vue file cannot be imported; keep in step with it)
function viewer(schemaStr, eventCode) {
	let parsed = null;
	if (schemaStr) {
		try { parsed = JSON.parse(schemaStr); } catch { parsed = null; }
	}
	const guard = (none, fail, fn) => {
		if (!parsed) return none;
		try { return fn(); } catch { return fail; }
	};
	return {
		schema: !parsed ? schemaStr ?? "" : JSON.stringify(parsed, null, 2),
		example: guard("// Unable to generate example", "// Unable to generate example",
			() => JSON.stringify(codegen.generateExample(parsed), null, 2)),
		typescript: guard("// No schema available", "// Unable to generate TypeScript interface",
			() => codegen.generateTypeScriptInterface(parsed, eventCode)),
		php: guard("// No schema available", "// Unable to generate PHP DTO",
			() => codegen.generatePhpDto(parsed, eventCode)),
		python: guard("# No schema available", "# Unable to generate Python dataclass",
			() => codegen.generatePythonDataclass(parsed, eventCode)),
		java: guard("// No schema available", "// Unable to generate Java record",
			() => codegen.generateJavaRecord(parsed, eventCode)),
	};
}

// The TS highlightJson does not HTML-escape its input (an XSS the Go port
// fixes). The Go port escapes & < > first; reproduce that on the input so the
// TS function stays the oracle for tokenising.
const escJson = (s) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");

const golden = path.join(testdata, "golden");
rmSync(golden, { recursive: true, force: true });
mkdirSync(path.join(golden, "export"), { recursive: true });

const DEFAULT_CODE = "platform:iam:user:created";
for (const file of readdirSync(path.join(testdata, "schemas")).sort()) {
	if (!file.endsWith(".json")) continue;
	const name = file.slice(0, -5);
	const schemaStr = readFileSync(path.join(testdata, "schemas", file), "utf8");
	let code = DEFAULT_CODE;
	try { code = readFileSync(path.join(testdata, "schemas", name + ".code"), "utf8"); } catch {}
	const v = viewer(schemaStr, code);
	const out = {
		...v,
		"hl-schema": hl.highlightJson(escJson(v.schema)),
		"hl-example": hl.highlightJson(escJson(v.example)),
		"hl-typescript": hl.highlightTypeScript(v.typescript),
		"hl-php": hl.highlightPhp(v.php),
		"hl-python": hl.highlightPython(v.python),
		"hl-java": hl.highlightJava(v.java),
	};
	for (const [tab, text] of Object.entries(out)) {
		writeFileSync(path.join(golden, `${name}.${tab}.txt`), text);
	}
}

// ---- export ---------------------------------------------------------------
let captured = null;
globalThis.document = { createElement: () => ({ click() {} }) };
URL.createObjectURL = (blob) => { captured = blob; return "blob:test"; };
URL.revokeObjectURL = () => {};

for (const file of readdirSync(path.join(testdata, "export")).sort()) {
	if (!file.endsWith(".json")) continue;
	const input = JSON.parse(readFileSync(path.join(testdata, "export", file), "utf8"));
	const ets = input.map((e) => ({
		application: e.application, subdomain: e.subdomain, aggregate: e.aggregate, event: e.event,
		specVersions: e.currentSchema === ""
			? [{ status: "FINALISING", schema: "{}" }]
			: [{ status: "DEPRECATED", schema: "{}" }, { status: "CURRENT", schema: e.currentSchema }],
	}));
	captured = null;
	const result = exporter.exportSchemasAsZip(ets);
	const files = {};
	let order = [];
	if (captured) {
		const entries = unzipSync(new Uint8Array(await captured.arrayBuffer()));
		order = Object.keys(entries);
		for (const k of [...order].sort()) files[k] = strFromU8(entries[k]);
	}
	writeFileSync(
		path.join(golden, "export", file),
		JSON.stringify({ exported: result.exported, errors: result.errors, order, files }, null, 2) + "\n",
	);
}
console.log("goldens written to", golden);
