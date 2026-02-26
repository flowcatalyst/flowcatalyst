import { defineConfig } from "drizzle-kit";
import { config } from "dotenv";

config();

export default defineConfig({
	schema: "./src/platform/infrastructure/persistence/schema/index.ts",
	out: "./drizzle",
	dialect: "postgresql",
	dbCredentials: {
		url: process.env.DATABASE_URL || "postgres://localhost:5432/flowcatalyst",
	},
});
