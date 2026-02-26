import { defineConfig } from "@hey-api/openapi-ts";

const openApiInput =
	process.env.OPENAPI_LIVE === "true"
		? "http://localhost:8080/q/openapi"
		: "../flowcatalyst/openapi/openapi.yaml";

export default defineConfig({
	input: openApiInput,
	output: {
		path: "src/api/generated",
		format: false,
	},
	plugins: ["@hey-api/typescript", "@hey-api/sdk", "@hey-api/client-fetch"],
});
