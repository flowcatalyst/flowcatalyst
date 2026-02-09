import { defineConfig } from 'tsup';

export default defineConfig({
	entry: ['src/index.ts'],
	format: ['esm'],
	dts: true,
	target: 'node22',
	outDir: 'dist',
	clean: true,
	sourcemap: true,
});
