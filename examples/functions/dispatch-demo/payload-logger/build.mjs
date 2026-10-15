// Bundles src/index.ts into the single IIFE script the function runner's
// shared JS engine loads (same esbuild settings as @flowcatalyst/fn's
// fc-fn-build). `fcdev fn build|deploy <dir>` runs this through `npm run build`.
//
// APP_CODE (env, default "dispatchdemo") is the code of the application that
// owns the demo event type; it is baked into the bundle so the event type's
// first segment matches the function's application (the platform refuses an
// emit of a type the function's application does not own).
import { build } from "esbuild";

const appCode = process.env.APP_CODE || "dispatchdemo";
await build({
  entryPoints: ["src/index.ts"],
  bundle: true,
  format: "iife",
  platform: "neutral",
  target: "es2020",
  outfile: "dist/function.js",
  define: { __APP_CODE__: JSON.stringify(appCode) },
  logLevel: "info",
});
