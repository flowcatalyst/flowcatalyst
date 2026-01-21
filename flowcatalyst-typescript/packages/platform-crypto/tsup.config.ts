import { defineConfig } from 'tsup';

export default defineConfig({
	entry: ['src/index.ts'],
	format: ['esm'],
	dts: true,
	clean: true,
	sourcemap: true,
	// AWS SDK packages are optional - only imported if enabled
	external: ['@aws-sdk/client-secrets-manager', '@aws-sdk/client-ssm'],
});
