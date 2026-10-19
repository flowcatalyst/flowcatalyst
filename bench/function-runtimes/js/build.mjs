import { build } from "esbuild";
// Same settings as clients/fn-ts/bin/fc-fn-build.js.
await build({ entryPoints: ["workloads.ts"], bundle: true, format: "iife", platform: "neutral", mainFields: ["module", "main"], target: "es2020", outfile: "dist/function.js", logLevel: "info" });
