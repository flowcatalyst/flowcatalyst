import { defineConfig } from 'vitest/config';

export default defineConfig({
	esbuild: {
		target: 'node22',
	},
	test: {
		globals: true,
		environment: 'node',
		include: ['**/*.test.ts', '**/*.spec.ts'],
		exclude: ['**/node_modules/**', '**/dist/**'],
		coverage: {
			provider: 'v8',
			reporter: ['text', 'json', 'html'],
			exclude: ['**/node_modules/**', '**/dist/**', '**/*.test.ts', '**/*.spec.ts'],
		},
		testTimeout: 10000,
		hookTimeout: 10000,
	},
});
