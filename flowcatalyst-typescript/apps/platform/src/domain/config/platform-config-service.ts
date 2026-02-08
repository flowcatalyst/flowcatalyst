/**
 * Platform Config Service
 *
 * Business logic for platform configuration with scope-based access
 * control and client-to-global fallback.
 */

import type {
	PlatformConfig,
	ConfigScope,
	ConfigValueType,
} from './platform-config.js';
import { createPlatformConfig } from './platform-config.js';

/**
 * Dependencies for the platform config service.
 */
export interface PlatformConfigServiceDeps {
	readonly configRepository: {
		findByKey(
			applicationCode: string,
			section: string,
			property: string,
			scope: ConfigScope,
			clientId: string | null,
		): Promise<PlatformConfig | undefined>;
		findBySection(
			applicationCode: string,
			section: string,
			scope: ConfigScope,
			clientId: string | null,
		): Promise<PlatformConfig[]>;
		findByApplication(
			applicationCode: string,
			scope: ConfigScope,
			clientId: string | null,
		): Promise<PlatformConfig[]>;
		findById(id: string): Promise<PlatformConfig | undefined>;
		insert(entity: ReturnType<typeof createPlatformConfig>): Promise<PlatformConfig>;
		update(entity: PlatformConfig): Promise<PlatformConfig>;
		deleteByKey(
			applicationCode: string,
			section: string,
			property: string,
			scope: ConfigScope,
			clientId: string | null,
		): Promise<boolean>;
	};
	readonly accessRepository: {
		findByRoleCodes(
			applicationCode: string,
			roleCodes: readonly string[],
		): Promise<Array<{ canRead: boolean; canWrite: boolean }>>;
	};
}

const ADMIN_ROLE_PATTERN = /platform.*admin|admin.*platform/i;

/**
 * Platform config service.
 */
export interface PlatformConfigService {
	getValue(
		applicationCode: string,
		section: string,
		property: string,
		scope: ConfigScope,
		clientId: string | null,
	): Promise<string | undefined>;

	getValueWithFallback(
		applicationCode: string,
		section: string,
		property: string,
		clientId: string,
	): Promise<string | undefined>;

	getSection(
		applicationCode: string,
		section: string,
		scope: ConfigScope,
		clientId: string | null,
	): Promise<Map<string, string>>;

	getSectionWithFallback(
		applicationCode: string,
		section: string,
		clientId: string,
	): Promise<Map<string, string>>;

	setValue(params: {
		applicationCode: string;
		section: string;
		property: string;
		scope: ConfigScope;
		clientId: string | null;
		value: string;
		valueType: ConfigValueType;
		description: string | null;
	}): Promise<PlatformConfig>;

	deleteValue(
		applicationCode: string,
		section: string,
		property: string,
		scope: ConfigScope,
		clientId: string | null,
	): Promise<boolean>;

	getConfigs(
		applicationCode: string,
		scope: ConfigScope,
		clientId: string | null,
	): Promise<PlatformConfig[]>;

	getById(id: string): Promise<PlatformConfig | undefined>;

	canAccess(
		applicationCode: string,
		userRoles: readonly string[],
		isWrite: boolean,
	): Promise<boolean>;
}

/**
 * Create a platform config service.
 */
export function createPlatformConfigService(deps: PlatformConfigServiceDeps): PlatformConfigService {
	const { configRepository, accessRepository } = deps;

	return {
		async getValue(
			applicationCode: string,
			section: string,
			property: string,
			scope: ConfigScope,
			clientId: string | null,
		): Promise<string | undefined> {
			const config = await configRepository.findByKey(
				applicationCode,
				section,
				property,
				scope,
				clientId,
			);
			return config?.value;
		},

		async getValueWithFallback(
			applicationCode: string,
			section: string,
			property: string,
			clientId: string,
		): Promise<string | undefined> {
			// Try client-specific first
			const clientConfig = await configRepository.findByKey(
				applicationCode,
				section,
				property,
				'CLIENT',
				clientId,
			);
			if (clientConfig) return clientConfig.value;

			// Fall back to global
			const globalConfig = await configRepository.findByKey(
				applicationCode,
				section,
				property,
				'GLOBAL',
				null,
			);
			return globalConfig?.value;
		},

		async getSection(
			applicationCode: string,
			section: string,
			scope: ConfigScope,
			clientId: string | null,
		): Promise<Map<string, string>> {
			const configs = await configRepository.findBySection(
				applicationCode,
				section,
				scope,
				clientId,
			);

			const result = new Map<string, string>();
			for (const config of configs) {
				result.set(config.property, config.value);
			}
			return result;
		},

		async getSectionWithFallback(
			applicationCode: string,
			section: string,
			clientId: string,
		): Promise<Map<string, string>> {
			// Get global values first
			const globalConfigs = await configRepository.findBySection(
				applicationCode,
				section,
				'GLOBAL',
				null,
			);

			const result = new Map<string, string>();
			for (const config of globalConfigs) {
				result.set(config.property, config.value);
			}

			// Override with client-specific values
			const clientConfigs = await configRepository.findBySection(
				applicationCode,
				section,
				'CLIENT',
				clientId,
			);
			for (const config of clientConfigs) {
				result.set(config.property, config.value);
			}

			return result;
		},

		async setValue(params: {
			applicationCode: string;
			section: string;
			property: string;
			scope: ConfigScope;
			clientId: string | null;
			value: string;
			valueType: ConfigValueType;
			description: string | null;
		}): Promise<PlatformConfig> {
			const existing = await configRepository.findByKey(
				params.applicationCode,
				params.section,
				params.property,
				params.scope,
				params.clientId,
			);

			if (existing) {
				return configRepository.update({
					...existing,
					value: params.value,
					valueType: params.valueType,
					description: params.description ?? existing.description,
				});
			}

			const entity = createPlatformConfig(params);
			return configRepository.insert(entity);
		},

		async deleteValue(
			applicationCode: string,
			section: string,
			property: string,
			scope: ConfigScope,
			clientId: string | null,
		): Promise<boolean> {
			return configRepository.deleteByKey(applicationCode, section, property, scope, clientId);
		},

		async getConfigs(
			applicationCode: string,
			scope: ConfigScope,
			clientId: string | null,
		): Promise<PlatformConfig[]> {
			return configRepository.findByApplication(applicationCode, scope, clientId);
		},

		async getById(id: string): Promise<PlatformConfig | undefined> {
			return configRepository.findById(id);
		},

		async canAccess(
			applicationCode: string,
			userRoles: readonly string[],
			isWrite: boolean,
		): Promise<boolean> {
			// Platform admins always have access
			if (userRoles.some((role) => ADMIN_ROLE_PATTERN.test(role))) {
				return true;
			}

			const grants = await accessRepository.findByRoleCodes(applicationCode, userRoles);

			if (isWrite) {
				return grants.some((grant) => grant.canWrite);
			}
			return grants.some((grant) => grant.canRead);
		},
	};
}
