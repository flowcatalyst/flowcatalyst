// Builds the one WebAuthn bundle the passkey pages load (see docs/ui/README.md,
// "Sign-in pages, part B"). Run with `npm ci && npm run build` in this
// directory, only when upgrading a pinned version. The bundle and the licence
// of what it contains are written into internal/ui/assets/static and committed.
import { build } from "esbuild";
import { readFileSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const here = fileURLToPath(new URL(".", import.meta.url));
const out = fileURLToPath(new URL("../../internal/ui/assets/static/", import.meta.url));

const result = await build({
  entryPoints: [here + "src/fc-webauthn.js"],
  outfile: out + "fc-webauthn.js",
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
  const licence = readFileSync(dir + "LICENSE.md", "utf8").trim();
  return `## ${name} ${pkg.version} (${pkg.license})\n\n${licence}\n`;
});
writeFileSync(
  out + "LICENSE-webauthn.md",
  "# @simplewebauthn/browser (bundled in fc-webauthn.js)\n\n" +
    "Built by tools/vendor-webauthn/build.mjs; the licence of each bundled package follows.\n\n" +
    notices.join("\n"),
);
