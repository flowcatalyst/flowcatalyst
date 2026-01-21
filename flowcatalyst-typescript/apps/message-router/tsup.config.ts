import { defineConfig } from 'tsup';

export default defineConfig({
	entry: ['src/index.ts'],
	format: ['esm'],
	dts: false, // App doesn't need declaration files
	clean: true,
	sourcemap: true,
	target: 'node22',
	splitting: false,
	// Bundle dependencies for deployment
	noExternal: [/@flowcatalyst\/.*/],
});
