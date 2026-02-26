import { defineConfig } from "tsup";

export default defineConfig({
	entry: ["src/index.ts"],
	format: ["esm"],
	dts: false,
	clean: true,
	sourcemap: true,
	target: "node24",
	external: [
		"node:sea",
		"@aws-sdk/client-secrets-manager",
		"@aws-sdk/client-ssm",
		// Native addons / worker-thread loaders / WASM — can't be bundled by esbuild
		"pino",
		"pino-pretty",
		"argon2",
		"nodemailer",
		"stompit",
		"sql.js",
		"oidc-provider",
	],
	esbuildOptions(options) {
		options.alias = {
			"@flowcatalyst/tsid": "../../packages/tsid/src/index.ts",
			"@flowcatalyst/logging": "../../packages/logging/src/index.ts",
			"@flowcatalyst/config": "../../packages/config/src/index.ts",
			"@flowcatalyst/contracts": "../../packages/contracts/src/index.ts",
			"@flowcatalyst/platform-crypto": "../../packages/crypto/src/index.ts",
			"@flowcatalyst/domain": "../../packages/domain/src/index.ts",
			"@flowcatalyst/application": "../../packages/framework/src/index.ts",
			"@flowcatalyst/http": "../../packages/http/src/index.ts",
			"@flowcatalyst/persistence": "../../packages/persistence/src/index.ts",
			"@flowcatalyst/queue-core": "../../packages/queue-core/src/index.ts",
			"@flowcatalyst/platform": "../../packages/platform/src/index.ts",
			"@flowcatalyst/message-router": "../../packages/message-router/src/index.ts",
			"@flowcatalyst/stream-processor": "../../packages/stream-processor/src/index.ts",
			"@flowcatalyst/outbox-processor": "../../packages/outbox-processor/src/index.ts",
		};
	},
});
