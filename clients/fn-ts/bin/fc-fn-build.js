#!/usr/bin/env node
// Bundles a FlowCatalyst JavaScript function's entry point into the single
// IIFE script the shared QuickJS engine loads (docs/function-runner-plan.md
// §2.2/§9; clients/fn-js-engine/src/lib.rs's module doc). The entry point
// must import "@flowcatalyst/fn" and register its endpoints at module top
// level; the bundle it produces is what `fn publish`/`fn deploy` upload.
//
// Usage: fc-fn-build <entry.ts> [--outfile dist/function.js]
import { build } from "esbuild";
import { resolve } from "node:path";

function parseArgs(argv) {
  const rest = [];
  let outfile;
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === "--outfile" || a === "-o") {
      outfile = argv[++i];
    } else {
      rest.push(a);
    }
  }
  return { entry: rest[0], outfile };
}

const { entry, outfile } = parseArgs(process.argv.slice(2));
if (!entry) {
  console.error("usage: fc-fn-build <entry.ts> [--outfile dist/function.js]");
  process.exit(1);
}

const out = outfile ?? "dist/function.js";

try {
  await build({
    entryPoints: [resolve(entry)],
    bundle: true,
    format: "iife",
    platform: "neutral",
    target: "es2020",
    outfile: resolve(out),
    logLevel: "info",
  });
} catch (err) {
  console.error(String(err));
  process.exit(1);
}
