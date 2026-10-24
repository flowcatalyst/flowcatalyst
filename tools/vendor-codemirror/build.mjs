// Builds the one CodeMirror bundle the /ui pages load (see docs/ui/README.md,
// "Code editor"). Run with `npm ci && npm run build` in this directory, only
// when upgrading a pinned version. The bundle and the licences of what it
// contains are written into internal/ui/assets/static and committed.
import { build } from "esbuild";
import { readFileSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const here = fileURLToPath(new URL(".", import.meta.url));
const out = fileURLToPath(new URL("../../internal/ui/assets/static/", import.meta.url));

const result = await build({
  entryPoints: [here + "src/fc-code-editor.js"],
  outfile: out + "fc-code-editor.js",
  bundle: true,
  minify: true,
  format: "iife",
  target: "es2022",
  legalComments: "none",
  metafile: true,
  logLevel: "info",
});

// The licence of every package that ended up in the bundle, once each.
const packages = new Map();
for (const input of Object.keys(result.metafile.inputs)) {
  const m = input.match(/^(.*node_modules\/((?:@[^/]+\/)?[^/]+))\//);
  if (m) packages.set(m[2], m[1] + "/");
}
const notices = [...packages.keys()].sort().map((name) => {
  const dir = here + packages.get(name);
  const pkg = JSON.parse(readFileSync(dir + "package.json", "utf8"));
  const licence = readFileSync(dir + "LICENSE", "utf8").trim();
  return `## ${name} ${pkg.version} (${pkg.license})\n\n${licence}\n`;
});
writeFileSync(
  out + "LICENSE-codemirror.md",
  "# CodeMirror 6 and Lezer (bundled in fc-code-editor.js)\n\n" +
    "Built by tools/vendor-codemirror/build.mjs; the licence of each bundled package follows.\n\n" +
    notices.join("\n"),
);
