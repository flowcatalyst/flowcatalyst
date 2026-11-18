<?php

declare(strict_types=1);

namespace FlowCatalyst\Client;

use FlowCatalyst\Client\Auth\OidcTokenManager;
use FlowCatalyst\Client\Auth\TokenProviderInterface;
use FlowCatalyst\Client\Auth\UserTokenProvider;
use FlowCatalyst\Client\Resources\Applications;
use FlowCatalyst\Client\Resources\AuditLogs;
use FlowCatalyst\Client\Resources\Clients;
use FlowCatalyst\Client\Resources\DispatchJobs;
use FlowCatalyst\Client\Resources\DispatchPools;
use FlowCatalyst\Client\Resources\EmailDomainMappings;
use FlowCatalyst\Client\Resources\EventTypes;
use FlowCatalyst\Client\Resources\FieldDevices;
use FlowCatalyst\Client\Resources\FieldPolicies;
use FlowCatalyst\Client\Resources\FieldTokens;
use FlowCatalyst\Client\Resources\FieldWorkers;
use FlowCatalyst\Client\Resources\IngestionSets;
use FlowCatalyst\Client\Resources\Ingestions;
use FlowCatalyst\Client\Resources\Me;
use FlowCatalyst\Client\Resources\Permissions;
use FlowCatalyst\Client\Resources\PlatformConfig;
use FlowCatalyst\Client\Resources\Principals;
use FlowCatalyst\Client\Resources\Processes;
use FlowCatalyst\Client\Resources\Roles;
use FlowCatalyst\Client\Resources\Connections;
use FlowCatalyst\Client\Resources\Router;
use FlowCatalyst\Client\Resources\ScheduledJobs;
use FlowCatalyst\Client\Resources\Subscriptions;
use FlowCatalyst\Exceptions\AuthenticationException;
use FlowCatalyst\Exceptions\FlowCatalystException;
use FlowCatalyst\Exceptions\OutcomeUnknownException;
use FlowCatalyst\Exceptions\ValidationException;
use FlowCatalyst\Generated\Client as GeneratedClient;
use FlowCatalyst\Sync\DefinitionSynchronizer;
use GuzzleHttp\Client;
use GuzzleHttp\Exception\ConnectException;
use GuzzleHttp\Exception\GuzzleException;
use GuzzleHttp\Exception\RequestException;
use GuzzleHttp\Exception\TransferException;
use Psr\Http\Message\ResponseInterface;

class FlowCatalystClient
{
    /** Key under which request() returns a 2xx body that is not JSON. */
    public const RAW_BODY = '_raw';

    private Client $httpClient;
    private TokenProviderInterface $tokenProvider;
    private readonly string $baseUrl;
    private ?GeneratedClient $generatedClient = null;
    private ?EventTypes $eventTypes = null;
    private ?Subscriptions $subscriptions = null;
    private ?DispatchPools $dispatchPools = null;
    private ?DispatchJobs $dispatchJobs = null;
    private ?Roles $roles = null;
    private ?Permissions $permissions = null;
    private ?Applications $applications = null;
    private ?Clients $clients = null;
    private ?Principals $principals = null;
    private ?Processes $processes = null;
    private ?Connections $connections = null;
    private ?Ingestions $ingestions = null;
    private ?IngestionSets $ingestionSets = null;
    private ?Me $me = null;
    private ?Router $router = null;
    private ?ScheduledJobs $scheduledJobs = null;
    private ?AuditLogs $auditLogs = null;
    private ?PlatformConfig $platformConfig = null;
    private ?EmailDomainMappings $emailDomainMappings = null;
    private ?DefinitionSynchronizer $definitions = null;
    private ?Admin $admin = null;
    private ?FieldWorkers $fieldWorkers = null;
    private ?FieldDevices $fieldDevices = null;
    private ?FieldPolicies $fieldPolicies = null;
    private ?FieldTokens $fieldTokens = null;

    /**
     * Create a new FlowCatalyst client.
     *
     * Both $tokenProvider and $baseUrl fall back to config when omitted, so the
     * container can autowire this class. The union type was dropped to a plain
     * interface because Laravel's container cannot autowire a union-typed
     * parameter (it can't pick which member to resolve), which silently broke
     * resolution when this client was resolved without an explicit binding.
     *
     * @param TokenProviderInterface|null $tokenProvider Token provider for authentication (defaults to a config-driven OidcTokenManager)
     * @param string|null $baseUrl Base URL of the FlowCatalyst API (defaults to config('flowcatalyst.base_url'))
     * @param int $timeout Request timeout in seconds
     * @param int $retryAttempts Total attempts for a transient failure (1 turns retries off). What is
     *                            retried follows {@see self::request()}: 408, 429 and a connection never
     *                            made for every request; 502, 503, 504 and a transport failure after
     *                            sending only for a repeatable one. Any other status is thrown on the
     *                            first attempt.
     * @param int $retryDelay Base delay between retries in milliseconds
     */
    public function __construct(
        ?TokenProviderInterface $tokenProvider = null,
        ?string $baseUrl = null,
        private readonly int $timeout = 30,
        private readonly int $retryAttempts = 3,
        private readonly int $retryDelay = 100,
        private readonly ?string $routerBaseUrl = null
    ) {
        $this->tokenProvider = $tokenProvider ?? new OidcTokenManager();
        $this->baseUrl = $baseUrl ?? (string) config('flowcatalyst.base_url', '');
        $this->httpClient = new Client([
            'base_uri' => rtrim($this->baseUrl, '/'),
            'timeout' => $this->timeout,
            'http_errors' => false,
            // strict: true preserves the request method across 3xx redirects.
            // Guzzle's default follows 301/302 with GET (per RFC 7231), so a
            // POST through an intermediary that normalises hostnames/paths
            // silently becomes a GET — the platform then returns 405 on
            // write-only routes.
            'allow_redirects' => ['max' => 5, 'strict' => true],
        ]);
    }

    /**
     * Create a client with a user access token.
     *
     * Use this when you already have a user access token (e.g., from OIDC login)
     * and want to make API calls on behalf of that user.
     *
     * @param string|\Closure(): string $token The access token or a callable that returns it
     * @param string $baseUrl Base URL of the FlowCatalyst API
     * @param int $timeout Request timeout in seconds
     * @param int $retryAttempts Total attempts for a transient failure (1 turns retries off). What is
     *                            retried follows {@see self::request()}: 408, 429 and a connection never
     *                            made for every request; 502, 503, 504 and a transport failure after
     *                            sending only for a repeatable one. Any other status is thrown on the
     *                            first attempt.
     * @param int $retryDelay Base delay between retries in milliseconds
     */
    public static function withUserToken(
        string|\Closure $token,
        string $baseUrl,
        int $timeout = 30,
        int $retryAttempts = 3,
        int $retryDelay = 100
    ): self {
        return new self(
            new UserTokenProvider($token),
            $baseUrl,
            $timeout,
            $retryAttempts,
            $retryDelay
        );
    }

    /**
     * Administrative surface: principals, clients, applications and router
     * checks. The old accessors (`principals()`, `clients()`, `applications()`,
     * `router()`) still work and raise a once-per-process deprecation notice
     * for the methods that live here.
     */
    public function admin(): Admin
    {
        return $this->admin ??= new Admin($this);
    }

    /**
     * Field workers (PIN identity plane): ensure, list, get, update, delete,
     * resetPin, unlock, suspend, activate, grantApp, revokeApp, signIns.
     */
    public function fieldWorkers(): FieldWorkers
    {
        return $this->fieldWorkers ??= new FieldWorkers($this);
    }

    /**
     * Field devices: createEnrolmentCode, list, get, suspend, activate, revoke.
     */
    public function fieldDevices(): FieldDevices
    {
        return $this->fieldDevices ??= new FieldDevices($this);
    }

    /**
     * Per-client field policy: get, put.
     */
    public function fieldPolicies(): FieldPolicies
    {
        return $this->fieldPolicies ??= new FieldPolicies($this);
    }

    /**
     * Field token introspection (`introspect($token)`).
     */
    public function fieldTokens(): FieldTokens
    {
        return $this->fieldTokens ??= new FieldTokens($this);
    }

    /**
     * Online check of a field access token; shortcut for
     * `fieldTokens()->introspect($token)`.
     */
    public function introspectFieldToken(string $token): ?\FlowCatalyst\Generated\Model\FieldTokenIntrospectResponse
    {
        return $this->fieldTokens()->introspect($token);
    }

    /**
     * Get the Ingestions resource (ingestions, versions, source kinds, test connection).
     */
    public function ingestions(): Ingestions
    {
        return $this->ingestions ??= new Ingestions($this);
    }

    /**
     * Get the Ingestion Sets resource.
     */
    public function ingestionSets(): IngestionSets
    {
        return $this->ingestionSets ??= new IngestionSets($this);
    }

    /**
     * Get the Event Types resource.
     */
    public function eventTypes(): EventTypes
    {
        return $this->eventTypes ??= new EventTypes($this);
    }

    /**
     * Get the Subscriptions resource.
     */
    public function subscriptions(): Subscriptions
    {
        return $this->subscriptions ??= new Subscriptions($this);
    }

    /**
     * Dispatch jobs created by calling the platform directly
     * (`batch(array $jobs)`, `create($job)`), not through the outbox.
     */
    public function dispatchJobs(): DispatchJobs
    {
        return $this->dispatchJobs ??= new DispatchJobs($this);
    }

    /**
     * Get the Connections resource.
     */
    public function connections(): Connections
    {
        return $this->connections ??= new Connections($this);
    }

    /**
     * Get the Dispatch Pools resource.
     */
    public function dispatchPools(): DispatchPools
    {
        return $this->dispatchPools ??= new DispatchPools($this);
    }

    /**
     * Get the Roles resource.
     */
    public function roles(): Roles
    {
        return $this->roles ??= new Roles($this);
    }

    /**
     * Get the Permissions resource.
     */
    public function permissions(): Permissions
    {
        return $this->permissions ??= new Permissions($this);
    }

    /**
     * Get the Applications resource.
     */
    public function applications(): Applications
    {
        return $this->applications ??= new Applications($this);
    }

    /**
     * Get the Clients resource.
     */
    public function clients(): Clients
    {
        return $this->clients ??= new Clients($this);
    }

    /**
     * Get the Principals resource.
     */
    public function principals(): Principals
    {
        return $this->principals ??= new Principals($this);
    }

    /**
     * Get the Processes resource (process documentation: CRUD + sync).
     */
    public function processes(): Processes
    {
        return $this->processes ??= new Processes($this);
    }

    /**
     * Get the Me resource (user-scoped access to clients and applications).
     *
     * Use this when making requests on behalf of a user to get only
     * the resources they have access to based on their scope.
     */
    public function me(): Me
    {
        return $this->me ??= new Me($this);
    }

    /**
     * Get the Router monitoring resource.
     *
     * Provides presence checks against the message router's in-pipeline
     * map (single id and batch). Talks to the router URL configured at
     * construction; falls back to the platform `baseUrl` when no router
     * URL is set.
     */
    public function router(): Router
    {
        return $this->router ??= new Router($this);
    }

    /**
     * Get the Scheduled Jobs resource (CRUD + state transitions + history
     * reads + the SDK callback paths used by `ScheduledJobRunner`).
     */
    public function scheduledJobs(): ScheduledJobs
    {
        return $this->scheduledJobs ??= new ScheduledJobs($this);
    }

    /**
     * Read-only queries against the platform's audit-log table.
     */
    public function auditLogs(): AuditLogs
    {
        return $this->auditLogs ??= new AuditLogs($this);
    }

    /**
     * Platform configuration properties (get / set / delete / list).
     */
    public function platformConfig(): PlatformConfig
    {
        return $this->platformConfig ??= new PlatformConfig($this);
    }

    /**
     * Email-domain mapping lookup (by domain).
     */
    public function emailDomainMappings(): EmailDomainMappings
    {
        return $this->emailDomainMappings ??= new EmailDomainMappings($this);
    }

    /**
     * Bulk synchronizer — push a `SyncDefinitionSet` (roles, event types,
     * subscriptions, dispatch pools, principals, processes) for a single
     * application in one orchestrated call. Mirrors the TypeScript SDK's
     * `client.definitions()`.
     */
    public function definitions(): DefinitionSynchronizer
    {
        return $this->definitions ??= new DefinitionSynchronizer($this);
    }

    /**
     * The base URL the router resource should target. Returns the
     * configured `routerBaseUrl` if set, otherwise the platform `baseUrl`.
     */
    public function getRouterBaseUrl(): string
    {
        return rtrim($this->routerBaseUrl ?? $this->baseUrl, '/');
    }

    /**
     * Get the JanePHP generated API client.
     */
    public function generated(): GeneratedClient
    {
        return $this->generatedClient ??= GeneratedClientFactory::create(
            $this->tokenProvider,
            $this->baseUrl
        );
    }

    /** Statuses that say the request was not processed: retried for every request. */
    private const REFUSED_STATUSES = [408, 429];

    /** Gateway statuses after which nobody can say whether the request was applied. */
    private const GATEWAY_STATUSES = [502, 503, 504];

    /** Methods a request of which may always be sent again. */
    private const REPEATABLE_METHODS = ['GET', 'HEAD', 'OPTIONS', 'PUT', 'DELETE'];

    /**
     * cURL errors of a connection that was never made, so nothing was sent:
     * could not resolve the proxy or host (5, 6), could not connect (7), TLS
     * handshake failed (35).
     */
    private const CURL_NOT_SENT = [5, 6, 7, 35];

    /**
     * Make an authenticated API request.
     *
     * A normal attempt uses the token provider's cached access token. Only a
     * 401 triggers a one-shot {@see TokenProviderInterface::refreshToken()}
     * on the first attempt, and only when the provider is an
     * {@see OidcTokenManager} (a plain user-token provider has nothing
     * useful to refresh); a second 401 surfaces as an
     * {@see AuthenticationException}.
     *
     * Retries, up to `retryAttempts` attempts in all, never refreshing the
     * token, follow whether the request is repeatable: its method is GET,
     * HEAD, OPTIONS, PUT or DELETE, or the caller passes `$repeatable` for a
     * POST the platform applies once however often it is sent (the
     * declarative syncs; a dispatch-job batch whose every job has an
     * idempotency key). Then:
     *
     * - 408 and 429 (refused before it was processed), and a connection that
     *   was never made (host not resolved, refused, TLS handshake failed),
     *   are retried for every request.
     * - 502, 503 and 504, and a transport failure after the request may have
     *   been sent (reset, empty reply, timeout), are retried for a repeatable
     *   request. Any other request is not sent again: it throws
     *   {@see OutcomeUnknownException} at once, because the platform may
     *   have applied it (a provisioning repeated after its answer was lost is
     *   refused 409 and its one-time secrets are gone; a create can make a
     *   duplicate).
     * - A DELETE that answers 404 on a retry after an attempt whose outcome
     *   was unknown succeeds (returns []): the earlier attempt most likely
     *   deleted it.
     *
     * @param bool $repeatable The request may be sent again after its answer was lost.
     *                         Only for a POST that is safe to send twice; GET, HEAD,
     *                         OPTIONS, PUT and DELETE are repeatable without it.
     *
     * @throws FlowCatalystException
     * @throws OutcomeUnknownException
     * @throws AuthenticationException
     * @throws ValidationException
     */
    public function request(string $method, string $endpoint, array $options = [], bool $repeatable = false): array
    {
        $canRefreshToken = $this->tokenProvider instanceof OidcTokenManager;
        $token = $this->tokenProvider->getAccessToken();
        $repeatable = $repeatable || in_array(strtoupper($method), self::REPEATABLE_METHODS, true);
        $attempt = 0;
        // An earlier attempt may have been applied by the platform.
        $uncertain = false;

        while (true) {
            try {
                return $this->doRequest($method, $endpoint, $options, $token);
            } catch (AuthenticationException $e) {
                if ($canRefreshToken && $attempt === 0) {
                    $token = $this->tokenProvider->refreshToken();
                    $attempt++;
                    continue;
                }

                throw $e;
            } catch (RetryableFailure $e) {
                if ($e->outcomeUnknown()) {
                    if (!$repeatable) {
                        $status = $e->kind === RetryableFailure::GATEWAY ? $e->exception->getCode() : null;
                        throw new OutcomeUnknownException(
                            strtoupper($method),
                            $endpoint,
                            $status,
                            $e->exception->getContext(),
                            $status === null ? $e->exception : null,
                        );
                    }
                    $uncertain = true;
                }
                if ($attempt + 1 < $this->retryAttempts) {
                    $attempt++;
                    usleep($this->retryDelay * 1000 * $attempt); // Exponential backoff
                    continue;
                }

                throw $e->exception;
            } catch (FlowCatalystException $e) {
                if ($uncertain && strtoupper($method) === 'DELETE' && $e->getCode() === 404) {
                    return [];
                }

                throw $e;
            }
        }
    }

    /**
     * Perform the actual HTTP request with the given (already resolved)
     * bearer token.
     */
    private function doRequest(string $method, string $endpoint, array $options, string $token): array
    {
        $options['headers'] = array_merge($options['headers'] ?? [], [
            'Authorization' => "Bearer {$token}",
            'Accept' => 'application/json',
            'Content-Type' => 'application/json',
        ]);

        // Convert body to JSON if it's an array
        if (isset($options['body']) && is_array($options['body'])) {
            $options['body'] = json_encode($options['body']);
        }

        // DEBUG: Log request details
        if (env('FLOWCATALYST_DEBUG', false)) {
            \Illuminate\Support\Facades\Log::debug("FLOWCATALYST REQUEST: {$method} {$endpoint}", [
                'body' => $options['body'] ?? $options['json'] ?? null,
            ]);
        }

        // Handle JSON body
        if (isset($options['json'])) {
            $options['body'] = json_encode($options['json']);
            unset($options['json']);
        }

        try {
            $response = $this->httpClient->request($method, $endpoint, $options);
        } catch (RequestException $e) {
            // The client is built with http_errors=false, so this only fires
            // for genuine transport failures (connect errors, timeouts).
            // Defensively check for an attached response anyway: a wrapped
            // 404 (or any status) must go through the normal status handling
            // below, never be mistaken for a network failure and retried.
            if ($e->hasResponse()) {
                return $this->handleResponse($e->getResponse());
            }

            throw new RetryableFailure(new FlowCatalystException(
                'Request failed: ' . $e->getMessage(),
                $e->getCode(),
                $e
            ), self::transportKind($e));
        } catch (GuzzleException $e) {
            throw new RetryableFailure(new FlowCatalystException(
                'HTTP client error: ' . $e->getMessage(),
                0,
                $e
            ), self::transportKind($e));
        }

        return $this->handleResponse($response);
    }

    /**
     * Whether a transport failure happened before anything was sent
     * (NOT_SENT) or after the request may have been (MAYBE_SENT). cURL says
     * which in the handler context: its error number, and `request_size`,
     * the bytes of request it wrote. Without a context (another handler), a
     * ConnectException is taken as not sent and anything else as maybe sent.
     */
    private static function transportKind(\Throwable $e): string
    {
        $context = $e instanceof TransferException && method_exists($e, 'getHandlerContext')
            ? $e->getHandlerContext()
            : [];
        if (isset($context['errno'])) {
            if (in_array((int) $context['errno'], self::CURL_NOT_SENT, true)) {
                return RetryableFailure::NOT_SENT;
            }
            if (array_key_exists('request_size', $context) && (int) $context['request_size'] === 0) {
                return RetryableFailure::NOT_SENT; // e.g. a connect timeout (28) before any byte
            }

            return RetryableFailure::MAYBE_SENT;
        }

        return $e instanceof ConnectException ? RetryableFailure::NOT_SENT : RetryableFailure::MAYBE_SENT;
    }

    /**
     * Translate an HTTP response into either the decoded payload or the
     * appropriate typed exception.
     *
     * Platform error responses look like:
     *   { "error": "<machine code>", "message": "<human text>" }
     * Surface the human message (falling back to the code) so callers
     * get "Cannot reset password for OIDC-authenticated users" instead
     * of "DUPLICATE".
     */
    private function handleResponse(ResponseInterface $response): array
    {
        $statusCode = $response->getStatusCode();
        $body = (string) $response->getBody();
        $data = json_decode($body, true) ?? [];

        // A 2xx answer that is not JSON (the ingestion definition is YAML) is
        // handed back whole under RAW_BODY.
        if ($statusCode >= 200 && $statusCode < 300 && $body !== '' && json_last_error() !== JSON_ERROR_NONE) {
            $data = [self::RAW_BODY => $body];
        }

        if ($statusCode === 401) {
            throw AuthenticationException::tokenExpired();
        }

        if ($statusCode === 403) {
            throw new FlowCatalystException(
                $data['message'] ?? $data['error'] ?? 'Access forbidden',
                403,
                null,
                $data
            );
        }

        if ($statusCode === 404) {
            throw new FlowCatalystException(
                $data['message'] ?? $data['error'] ?? 'Resource not found',
                404,
                null,
                $data
            );
        }

        if ($statusCode === 422) {
            throw ValidationException::fromResponse($data);
        }

        if ($statusCode >= 400 && $statusCode < 500) {
            $exception = new FlowCatalystException(
                $data['message'] ?? $data['error'] ?? "Client error: {$statusCode}",
                $statusCode,
                null,
                $data
            );

            throw in_array($statusCode, self::REFUSED_STATUSES, true)
                ? new RetryableFailure($exception, RetryableFailure::REFUSED)
                : $exception;
        }

        if ($statusCode >= 500) {
            $exception = new FlowCatalystException(
                $data['message'] ?? $data['error'] ?? "Server error: {$statusCode}",
                $statusCode,
                null,
                $data
            );

            throw in_array($statusCode, self::GATEWAY_STATUSES, true)
                ? new RetryableFailure($exception, RetryableFailure::GATEWAY)
                : $exception;
        }

        return $data;
    }

    /**
     * Get the base URL.
     */
    public function getBaseUrl(): string
    {
        return $this->baseUrl;
    }

    /**
     * Get the token provider.
     */
    public function getTokenProvider(): TokenProviderInterface
    {
        return $this->tokenProvider;
    }

    /**
     * Get the token manager (for backward compatibility).
     *
     * @deprecated Use getTokenProvider() instead
     * @throws \RuntimeException If the token provider is not an OidcTokenManager
     */
    public function getTokenManager(): OidcTokenManager
    {
        if (!$this->tokenProvider instanceof OidcTokenManager) {
            throw new \RuntimeException(
                'getTokenManager() is only available when using OidcTokenManager. Use getTokenProvider() instead.'
            );
        }

        return $this->tokenProvider;
    }
}
