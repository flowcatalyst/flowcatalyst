/**
 * Route registration — all Fastify route groups: admin, bff, sdk, batch,
 * me, public, debug, application-sync.
 */

import type { FastifyInstance } from "fastify";
import { jsonSuccess } from "@flowcatalyst/http";
import type { DrizzleUnitOfWorkConfig } from "@flowcatalyst/persistence";
import type {
	EncryptionService,
	PasswordService,
} from "@flowcatalyst/platform-crypto";
import {
	registerAdminRoutes,
	type AdminRoutesDeps,
	registerBffRoutes,
	type BffRoutesDeps,
	registerSdkRoutes,
	type SdkRoutesDeps,
	registerBatchRoutes,
	type BatchRoutesDeps,
	registerMeApiRoutes,
	type MeRoutesDeps,
	registerPublicApiRoutes,
	registerPlatformConfigApiRoutes,
	registerDebugBffRoutes,
	registerApplicationSyncApiRoutes,
	type ApplicationSyncRoutesDeps,
} from "../api/index.js";
import type { PlatformConfigService } from "../domain/index.js";
import type { Repositories } from "./repositories.js";
import type { UseCases } from "./use-cases.js";

export interface RegisterRoutesDeps {
	repos: Repositories;
	useCases: UseCases;
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	db: any;
	uowConfig: DrizzleUnitOfWorkConfig;
	platformConfigService: PlatformConfigService;
	passwordService: PasswordService;
	encryptionService: EncryptionService;
}

export async function registerPlatformRoutes(
	fastify: FastifyInstance,
	deps: RegisterRoutesDeps,
): Promise<void> {
	const {
		repos,
		useCases,
		db,
		uowConfig,
		platformConfigService,
		passwordService,
		encryptionService,
	} = deps;

	// Health check
	fastify.get("/health", async (request, reply) => {
		return jsonSuccess(reply, {
			status: "healthy",
			service: "platform",
			timestamp: new Date().toISOString(),
		});
	});

	// Admin API routes
	const adminDeps: AdminRoutesDeps = {
		// Principal management
		principalRepository: repos.principalRepository,
		clientAccessGrantRepository: repos.clientAccessGrantRepository,
		passwordService,
		...useCases,
		// Client management
		clientRepository: repos.clientRepository,
		// Anchor domain management
		anchorDomainRepository: repos.anchorDomainRepository,
		// Application management
		applicationRepository: repos.applicationRepository,
		applicationClientConfigRepository: repos.applicationClientConfigRepository,
		// Role management
		roleRepository: repos.roleRepository,
		permissionRepository: repos.permissionRepository,
		// Auth config management
		clientAuthConfigRepository: repos.clientAuthConfigRepository,
		// OAuth client management
		oauthClientRepository: repos.oauthClientRepository,
		// Audit log viewing
		auditLogRepository: repos.auditLogRepository,
		// EventType management
		eventTypeRepository: repos.eventTypeRepository,
		// Dispatch Pool management
		dispatchPoolRepository: repos.dispatchPoolRepository,
		// Subscription management
		subscriptionRepository: repos.subscriptionRepository,
		// Event & Dispatch Job read models
		eventReadRepository: repos.eventReadRepository,
		dispatchJobReadRepository: repos.dispatchJobReadRepository,
		// Identity Provider management
		identityProviderRepository: repos.identityProviderRepository,
		// Email Domain Mapping management
		emailDomainMappingRepository: repos.emailDomainMappingRepository,
		// CORS origin management
		corsAllowedOriginRepository: repos.corsAllowedOriginRepository,
		// Platform config management
		platformConfigService,
		platformConfigAccessRepository: repos.platformConfigAccessRepository,
		// Service Account management
		encryptionService,
		// Database (for OIDC client cache invalidation)
		db,
	};

	await registerAdminRoutes(fastify, adminDeps);

	// BFF routes (frontend-facing)
	const bffDeps: BffRoutesDeps = {
		// Event type BFF
		eventTypeRepository: repos.eventTypeRepository,
		...pick(useCases, [
			"createEventTypeUseCase",
			"updateEventTypeUseCase",
			"deleteEventTypeUseCase",
			"archiveEventTypeUseCase",
			"addSchemaUseCase",
			"finaliseSchemaUseCase",
			"deprecateSchemaUseCase",
			"createRoleUseCase",
			"updateRoleUseCase",
			"deleteRoleUseCase",
		]),
		// Role BFF
		roleRepository: repos.roleRepository,
		permissionRepository: repos.permissionRepository,
		applicationRepository: repos.applicationRepository,
		// Events & Dispatch Jobs BFF
		eventReadRepository: repos.eventReadRepository,
		dispatchJobReadRepository: repos.dispatchJobReadRepository,
	};

	await registerBffRoutes(fastify, bffDeps);

	// SDK routes (external integrations)
	const sdkDeps: SdkRoutesDeps = {
		// SDK Clients
		clientRepository: repos.clientRepository,
		...pick(useCases, [
			"createClientUseCase",
			"updateClientUseCase",
			"changeClientStatusUseCase",
			"deleteClientUseCase",
			"createRoleUseCase",
			"updateRoleUseCase",
			"deleteRoleUseCase",
			"createUserUseCase",
			"updateUserUseCase",
			"activateUserUseCase",
			"deactivateUserUseCase",
			"assignRolesUseCase",
			"grantClientAccessUseCase",
			"revokeClientAccessUseCase",
		]),
		// SDK Roles
		roleRepository: repos.roleRepository,
		applicationRepository: repos.applicationRepository,
		// SDK Principals
		principalRepository: repos.principalRepository,
		clientAccessGrantRepository: repos.clientAccessGrantRepository,
	};

	await registerSdkRoutes(fastify, sdkDeps);

	// Application-scoped sync routes (SDK sync endpoints)
	const applicationSyncDeps: ApplicationSyncRoutesDeps = {
		syncRolesUseCase: useCases.syncRolesUseCase,
		syncEventTypesUseCase: useCases.syncEventTypesUseCase,
		syncSubscriptionsUseCase: useCases.syncSubscriptionsUseCase,
		syncDispatchPoolsUseCase: useCases.syncDispatchPoolsUseCase,
		syncPrincipalsUseCase: useCases.syncPrincipalsUseCase,
	};

	await registerApplicationSyncApiRoutes(fastify, applicationSyncDeps);

	// Batch ingestion routes (outbox processor / SDK batch endpoints)
	const batchDeps: BatchRoutesDeps = {
		db,
		getPostCommitDispatcher: () => uowConfig.postCommitDispatch,
	};

	await registerBatchRoutes(fastify, batchDeps);

	// User-facing /api/me routes
	const meDeps: MeRoutesDeps = {
		clientRepository: repos.clientRepository,
		applicationRepository: repos.applicationRepository,
		applicationClientConfigRepository: repos.applicationClientConfigRepository,
	};

	await registerMeApiRoutes(fastify, meDeps);

	// Public routes (no auth required)
	const publicDeps = {
		platformConfigService,
	};

	await registerPublicApiRoutes(fastify, publicDeps);
	await registerPlatformConfigApiRoutes(fastify, publicDeps);

	// Debug BFF routes (raw event/dispatch job access)
	const debugBffDeps = {
		db,
	};

	await registerDebugBffRoutes(fastify, debugBffDeps);
}

/** Pick a subset of keys from an object. */
function pick<T extends object, K extends keyof T>(
	obj: T,
	keys: K[],
): Pick<T, K> {
	const result = {} as Pick<T, K>;
	for (const key of keys) {
		result[key] = obj[key];
	}
	return result;
}
