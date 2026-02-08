import { defineConfig } from 'tsup';

export default defineConfig([
	// Standard build (for `pnpm dev` and `node dist/index.js`)
	{
		entry: ['src/index.ts'],
		format: ['esm'],
		dts: false,
		clean: true,
		sourcemap: true,
		target: 'node22',
	},
]);
