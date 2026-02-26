/**
 * Messaging use cases — dispatch pools, subscriptions.
 */

import type { CreateUseCasesDeps } from "./index.js";
import {
	createGuardedUseCase,
	clientScopedGuard,
} from "../../authorization/index.js";
import {
	createCreateDispatchPoolUseCase,
	createUpdateDispatchPoolUseCase,
	createDeleteDispatchPoolUseCase,
	createSyncDispatchPoolsUseCase,
	createCreateSubscriptionUseCase,
	createUpdateSubscriptionUseCase,
	createDeleteSubscriptionUseCase,
	createSyncSubscriptionsUseCase,
} from "../../application/index.js";

export function createMessagingUseCases(deps: CreateUseCasesDeps) {
	const { repos, unitOfWork } = deps;

	// --- Dispatch Pool use cases (with client-scope guard for client-scoped pools) ---
	const createDispatchPoolUseCase = createGuardedUseCase(
		createCreateDispatchPoolUseCase({
			dispatchPoolRepository: repos.dispatchPoolRepository,
			clientRepository: repos.clientRepository,
			unitOfWork,
		}),
		clientScopedGuard(),
	);

	const updateDispatchPoolUseCase = createUpdateDispatchPoolUseCase({
		dispatchPoolRepository: repos.dispatchPoolRepository,
		unitOfWork,
	});

	const deleteDispatchPoolUseCase = createDeleteDispatchPoolUseCase({
		dispatchPoolRepository: repos.dispatchPoolRepository,
		unitOfWork,
	});

	const syncDispatchPoolsUseCase = createSyncDispatchPoolsUseCase({
		dispatchPoolRepository: repos.dispatchPoolRepository,
		unitOfWork,
	});

	// --- Subscription use cases (with client-scope guard for client-scoped subs) ---
	const createSubscriptionUseCase = createGuardedUseCase(
		createCreateSubscriptionUseCase({
			subscriptionRepository: repos.subscriptionRepository,
			dispatchPoolRepository: repos.dispatchPoolRepository,
			unitOfWork,
		}),
		clientScopedGuard(),
	);

	const updateSubscriptionUseCase = createUpdateSubscriptionUseCase({
		subscriptionRepository: repos.subscriptionRepository,
		dispatchPoolRepository: repos.dispatchPoolRepository,
		unitOfWork,
	});

	const deleteSubscriptionUseCase = createDeleteSubscriptionUseCase({
		subscriptionRepository: repos.subscriptionRepository,
		unitOfWork,
	});

	const syncSubscriptionsUseCase = createSyncSubscriptionsUseCase({
		subscriptionRepository: repos.subscriptionRepository,
		dispatchPoolRepository: repos.dispatchPoolRepository,
		unitOfWork,
	});

	return {
		createDispatchPoolUseCase,
		updateDispatchPoolUseCase,
		deleteDispatchPoolUseCase,
		syncDispatchPoolsUseCase,
		createSubscriptionUseCase,
		updateSubscriptionUseCase,
		deleteSubscriptionUseCase,
		syncSubscriptionsUseCase,
	};
}
