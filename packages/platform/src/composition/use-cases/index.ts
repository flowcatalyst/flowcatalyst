/**
 * Use case composition — assembles all per-domain use case factories.
 */

import type { UnitOfWork } from "@flowcatalyst/domain";
import type { PasswordService } from "@flowcatalyst/platform-crypto";
import type { EncryptionService } from "@flowcatalyst/platform-crypto";
import type { Repositories } from "../repositories.js";
import { createIamUseCases } from "./iam.js";
import { createTenancyUseCases } from "./tenancy.js";
import { createApplicationUseCases } from "./application.js";
import { createAuthConfigUseCases } from "./auth-config.js";
import { createOAuthUseCases } from "./oauth.js";
import { createEventTypeUseCases } from "./event-type.js";
import { createMessagingUseCases } from "./messaging.js";
import { createIdentityProviderUseCases } from "./identity-provider.js";
import { createEmailDomainUseCases } from "./email-domain.js";
import { createCorsUseCases } from "./cors.js";

export interface CreateUseCasesDeps {
	repos: Repositories;
	unitOfWork: UnitOfWork;
	passwordService: PasswordService;
	encryptionService: EncryptionService;
}

export function createUseCases(deps: CreateUseCasesDeps) {
	return {
		...createIamUseCases(deps),
		...createTenancyUseCases(deps),
		...createApplicationUseCases(deps),
		...createAuthConfigUseCases(deps),
		...createOAuthUseCases(deps),
		...createEventTypeUseCases(deps),
		...createMessagingUseCases(deps),
		...createIdentityProviderUseCases(deps),
		...createEmailDomainUseCases(deps),
		...createCorsUseCases(deps),
	};
}

export type UseCases = ReturnType<typeof createUseCases>;
