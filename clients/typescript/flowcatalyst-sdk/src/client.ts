/**
 * FlowCatalyst SDK Client
 *
 * Main client for interacting with the FlowCatalyst platform.
 * Uses neverthrow for typed error handling.
 */

import { ResultAsync, ok, err } from 'neverthrow';
import { createClient, createConfig } from './generated/client';
import type { Client } from './generated/client';
import { OidcTokenManager, type TokenManagerConfig } from './auth';
import type { SdkError } from './errors';
import { mapHttpStatusToError, httpError, authError } from './errors';

// Re-export resource classes
import { EventTypesResource } from './resources/event-types';
import { SubscriptionsResource } from './resources/subscriptions';
import { DispatchPoolsResource } from './resources/dispatch-pools';
import { RolesResource } from './resources/roles';
import { PermissionsResource } from './resources/permissions';
import { ApplicationsResource } from './resources/applications';
import { ClientsResource } from './resources/clients';
import { PrincipalsResource } from './resources/principals';

export interface FlowCatalystConfig extends TokenManagerConfig {
  /** Request timeout in milliseconds (default: 30000) */
  timeout?: number;
  /** Number of retry attempts for transient errors (default: 3) */
  retryAttempts?: number;
  /** Base delay between retries in ms (default: 100) */
  retryDelay?: number;
}

type ResolvedConfig = {
  baseUrl: string;
  clientId: string;
  clientSecret: string;
  tokenUrl?: string;
  timeout: number;
  retryAttempts: number;
  retryDelay: number;
};

/**
 * Main FlowCatalyst SDK client.
 *
 * @example
 * ```typescript
 * import { FlowCatalystClient } from '@flowcatalyst/sdk';
 *
 * const client = new FlowCatalystClient({
 *   baseUrl: 'https://your-instance.flowcatalyst.io',
 *   clientId: 'your_client_id',
 *   clientSecret: 'your_client_secret',
 * });
 *
 * // Using neverthrow Result pattern
 * const result = await client.eventTypes().list();
 * result.match(
 *   (eventTypes) => console.log('Event types:', eventTypes),
 *   (error) => console.error('Error:', error.type, error.message)
 * );
 * ```
 */
export class FlowCatalystClient {
  private readonly httpClient: Client;
  private readonly tokenManager: OidcTokenManager;
  private readonly config: ResolvedConfig;

  // Lazy-loaded resource instances
  private _eventTypes?: EventTypesResource;
  private _subscriptions?: SubscriptionsResource;
  private _dispatchPools?: DispatchPoolsResource;
  private _roles?: RolesResource;
  private _permissions?: PermissionsResource;
  private _applications?: ApplicationsResource;
  private _clients?: ClientsResource;
  private _principals?: PrincipalsResource;

  constructor(config: FlowCatalystConfig) {
    this.config = {
      timeout: 30000,
      retryAttempts: 3,
      retryDelay: 100,
      ...config,
      baseUrl: config.baseUrl.replace(/\/$/, ''),
    };

    this.tokenManager = new OidcTokenManager({
      baseUrl: this.config.baseUrl,
      clientId: this.config.clientId,
      clientSecret: this.config.clientSecret,
      tokenUrl: this.config.tokenUrl,
    });

    this.httpClient = createClient(
      createConfig({
        baseUrl: this.config.baseUrl,
      })
    );
  }

  // ============ Resource Accessors ============

  /** Event Types resource */
  eventTypes(): EventTypesResource {
    return (this._eventTypes ??= new EventTypesResource(this));
  }

  /** Subscriptions resource */
  subscriptions(): SubscriptionsResource {
    return (this._subscriptions ??= new SubscriptionsResource(this));
  }

  /** Dispatch Pools resource */
  dispatchPools(): DispatchPoolsResource {
    return (this._dispatchPools ??= new DispatchPoolsResource(this));
  }

  /** Roles resource */
  roles(): RolesResource {
    return (this._roles ??= new RolesResource(this));
  }

  /** Permissions resource */
  permissions(): PermissionsResource {
    return (this._permissions ??= new PermissionsResource(this));
  }

  /** Applications resource */
  applications(): ApplicationsResource {
    return (this._applications ??= new ApplicationsResource(this));
  }

  /** Clients resource */
  clients(): ClientsResource {
    return (this._clients ??= new ClientsResource(this));
  }

  /** Principals resource */
  principals(): PrincipalsResource {
    return (this._principals ??= new PrincipalsResource(this));
  }

  // ============ Internal Methods ============

  /**
   * Get the underlying HTTP client (for advanced usage).
   */
  getHttpClient(): Client {
    return this.httpClient;
  }

  /**
   * Get the token manager (for advanced usage).
   */
  getTokenManager(): OidcTokenManager {
    return this.tokenManager;
  }

  /**
   * Execute an authenticated request with retry logic.
   *
   * Wraps the generated SDK functions with:
   * - Automatic token injection
   * - Retry with exponential backoff
   * - Typed error handling via neverthrow
   */
  request<T>(
    fn: (client: Client, headers: Record<string, string>) => Promise<{ data?: unknown; error?: unknown; response: Response }>
  ): ResultAsync<T, SdkError> {
    return this.tokenManager
      .getAccessToken()
      .mapErr((e): SdkError => e)
      .andThen((token) => this.executeWithRetry<T>(fn, token, 0));
  }

  private executeWithRetry<T>(
    fn: (client: Client, headers: Record<string, string>) => Promise<{ data?: unknown; error?: unknown; response: Response }>,
    token: string,
    attempt: number
  ): ResultAsync<T, SdkError> {
    const headers = {
      Authorization: `Bearer ${token}`,
    };

    return ResultAsync.fromPromise(
      this.executeWithTimeout(fn, headers),
      (e) => {
        if (e instanceof Error && e.name === 'AbortError') {
          return httpError.timeout(this.config.timeout);
        }
        return httpError.network(e instanceof Error ? e.message : 'Network error', e instanceof Error ? e : undefined);
      }
    ).andThen((result) => {
      // Check if we got an error response
      if (result.error !== undefined || !result.response.ok) {
        const status = result.response.status;

        // Handle 401 with token refresh
        if (status === 401 && attempt === 0) {
          return this.tokenManager
            .refreshToken()
            .mapErr((e): SdkError => e)
            .andThen((newToken) => this.executeWithRetry<T>(fn, newToken, attempt + 1));
        }

        // Retry transient errors
        if (this.isRetryableStatus(status) && attempt < this.config.retryAttempts) {
          return ResultAsync.fromSafePromise<void, SdkError>(
            this.delay(this.config.retryDelay * Math.pow(2, attempt))
          ).andThen(() => this.executeWithRetry<T>(fn, token, attempt + 1));
        }

        const sdkError = mapHttpStatusToError(status, result.error);
        return err(sdkError);
      }

      return ok(result.data as T);
    });
  }

  private async executeWithTimeout(
    fn: (client: Client, headers: Record<string, string>) => Promise<{ data?: unknown; error?: unknown; response: Response }>,
    headers: Record<string, string>
  ): Promise<{ data?: unknown; error?: unknown; response: Response }> {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), this.config.timeout);

    try {
      // Note: The generated client doesn't support abort signals directly,
      // but we keep this pattern for future compatibility
      return await fn(this.httpClient, headers);
    } finally {
      clearTimeout(timeoutId);
    }
  }

  private isRetryableStatus(status: number): boolean {
    return [408, 429, 502, 503, 504].includes(status);
  }

  private delay(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }
}
