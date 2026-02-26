import { defineConfig } from "vitest/config";
import tsconfigPaths from "vite-tsconfig-paths";

export default defineConfig({
	plugins: [tsconfigPaths()],
	esbuild: {
		target: "node22",
	},
	test: {
		globals: true,
		environment: "node",
		include: ["src/**/*.test.ts", "src/**/*.spec.ts"],
		exclude: ["**/node_modules/**", "**/dist/**"],
		testTimeout: 10000,
		hookTimeout: 10000,
	},
});
