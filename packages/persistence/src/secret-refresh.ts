/**
 * Secret Refresh Manager
 *
 * Polls a SecretProvider on a fixed interval and calls onChanged when the
 * database URL changes. Used to support zero-downtime credential rotation:
 * AWS RDS and GCP Cloud SQL both support a dual-password rotation window
 * where the old password remains valid while the new one is active, giving
 * the application time to refresh before the old password is revoked.
 *
 * Enabled only when DB_SECRET_REFRESH_INTERVAL_MS > 0 and a non-env provider
 * is configured. No-ops for plain DATABASE_URL setups.
 */

import type { SecretProvider } from "./secret-provider.js";

export interface SecretRefreshOptions {
	provider: SecretProvider;
	/** Current database URL — compared against fetched value to detect changes. */
	currentUrl: string;
	/** Polling interval in ms. Set to 0 to disable. */
	intervalMs: number;
	/** Called when a new URL is detected. May be async — errors are caught and logged. */
	onChanged: (newUrl: string) => Promise<void>;
	logger: {
		info(msg: string, data?: Record<string, unknown>): void;
		warn(msg: string, data?: Record<string, unknown>): void;
		error(msg: string, data?: Record<string, unknown>): void;
	};
}

export interface SecretRefreshHandle {
	stop(): void;
}

export function startSecretRefresh(
	options: SecretRefreshOptions,
): SecretRefreshHandle {
	const { provider, intervalMs, onChanged, logger } = options;
	let currentUrl = options.currentUrl;
	let stopped = false;
	let timer: ReturnType<typeof setTimeout> | null = null;

	async function poll(): Promise<void> {
		if (stopped) return;

		try {
			const newUrl = await provider.getDbUrl();

			if (newUrl !== currentUrl) {
				logger.info(
					`DB credentials changed via ${provider.name} — refreshing connection pool`,
				);
				currentUrl = newUrl;
				try {
					await onChanged(newUrl);
					logger.info(`Connection pool refreshed successfully`);
				} catch (err) {
					logger.error(`Failed to refresh connection pool after credential change`, {
						err: err instanceof Error ? err.message : String(err),
					});
				}
			}
		} catch (err) {
			logger.warn(`Failed to poll ${provider.name} for credential changes`, {
				err: err instanceof Error ? err.message : String(err),
			});
		}

		if (!stopped) {
			timer = setTimeout(() => void poll(), intervalMs);
		}
	}

	// Schedule first poll after one interval (credentials were already fetched at startup)
	timer = setTimeout(() => void poll(), intervalMs);

	logger.info(`Secret refresh polling started`, {
		provider: provider.name,
		intervalMs,
	});

	return {
		stop() {
			stopped = true;
			if (timer) clearTimeout(timer);
		},
	};
}
