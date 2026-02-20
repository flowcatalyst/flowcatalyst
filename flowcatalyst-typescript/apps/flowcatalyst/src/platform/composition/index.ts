/**
 * Composition Root — barrel export for all composition modules.
 */

export { createRepositories, type Repositories } from "./repositories.js";
export { createPlatformAggregateRegistry } from "./aggregate-registry.js";
export {
	createDispatchInfrastructure,
	createPostCommitDispatcherFromPublisher,
	type DispatchInfrastructure,
	type CreateDispatchInfrastructureDeps,
} from "./dispatch.js";
export {
	createUseCases,
	type UseCases,
	type CreateUseCasesDeps,
} from "./use-cases.js";
export {
	registerPlatformPlugins,
	type RegisterPluginsDeps,
} from "./plugins.js";
export { registerPlatformRoutes, type RegisterRoutesDeps } from "./routes.js";
