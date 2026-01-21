<?php

declare(strict_types=1);

namespace FlowCatalyst\Client;

use FlowCatalyst\Client\Auth\OidcTokenManager;
use FlowCatalyst\Client\Resources\Applications;
use FlowCatalyst\Client\Resources\DispatchPools;
use FlowCatalyst\Client\Resources\EventTypes;
use FlowCatalyst\Client\Resources\Permissions;
use FlowCatalyst\Client\Resources\Roles;
use FlowCatalyst\Client\Resources\Subscriptions;
use FlowCatalyst\Exceptions\AuthenticationException;
use FlowCatalyst\Exceptions\FlowCatalystException;
use FlowCatalyst\Exceptions\ValidationException;
use GuzzleHttp\Client;
use GuzzleHttp\Exception\GuzzleException;
use GuzzleHttp\Exception\RequestException;

class FlowCatalystClient
{
    private Client $httpClient;
    private ?EventTypes $eventTypes = null;
    private ?Subscriptions $subscriptions = null;
    private ?DispatchPools $dispatchPools = null;
    private ?Roles $roles = null;
    private ?Permissions $permissions = null;
    private ?Applications $applications = null;

    public function __construct(
        private readonly OidcTokenManager $tokenManager,
        private readonly string $baseUrl,
        private readonly int $timeout = 30,
        private readonly int $retryAttempts = 3,
        private readonly int $retryDelay = 100
    ) {
        $this->httpClient = new Client([
            'base_uri' => rtrim($this->baseUrl, '/'),
            'timeout' => $this->timeout,
            'http_errors' => false,
        ]);
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
     * Make an authenticated API request.
     *
     * @throws FlowCatalystException
     * @throws AuthenticationException
     * @throws ValidationException
     */
    public function request(string $method, string $endpoint, array $options = []): array
    {
        $attempt = 0;
        $lastException = null;

        while ($attempt < $this->retryAttempts) {
            try {
                return $this->doRequest($method, $endpoint, $options, $attempt > 0);
            } catch (AuthenticationException $e) {
                // Don't retry auth failures
                throw $e;
            } catch (ValidationException $e) {
                // Don't retry validation errors
                throw $e;
            } catch (FlowCatalystException $e) {
                $lastException = $e;
                $attempt++;

                if ($attempt < $this->retryAttempts) {
                    usleep($this->retryDelay * 1000 * $attempt); // Exponential backoff
                }
            }
        }

        throw $lastException ?? new FlowCatalystException('Request failed after retries');
    }

    /**
     * Perform the actual HTTP request.
     */
    private function doRequest(string $method, string $endpoint, array $options, bool $isRetry): array
    {
        $token = $isRetry
            ? $this->tokenManager->refreshToken()
            : $this->tokenManager->getAccessToken();

        $options['headers'] = array_merge($options['headers'] ?? [], [
            'Authorization' => "Bearer {$token}",
            'Accept' => 'application/json',
            'Content-Type' => 'application/json',
        ]);

        // Convert body to JSON if it's an array
        if (isset($options['body']) && is_array($options['body'])) {
            $options['body'] = json_encode($options['body']);
        }

        // Handle JSON body
        if (isset($options['json'])) {
            $options['body'] = json_encode($options['json']);
            unset($options['json']);
        }

        try {
            $response = $this->httpClient->request($method, $endpoint, $options);
            $statusCode = $response->getStatusCode();
            $body = (string) $response->getBody();
            $data = json_decode($body, true) ?? [];

            // Handle different status codes
            if ($statusCode === 401) {
                throw AuthenticationException::tokenExpired();
            }

            if ($statusCode === 403) {
                throw new FlowCatalystException(
                    $data['error'] ?? 'Access forbidden',
                    403,
                    null,
                    $data
                );
            }

            if ($statusCode === 404) {
                throw new FlowCatalystException(
                    $data['error'] ?? 'Resource not found',
                    404,
                    null,
                    $data
                );
            }

            if ($statusCode === 422) {
                throw ValidationException::fromResponse($data);
            }

            if ($statusCode >= 400 && $statusCode < 500) {
                throw new FlowCatalystException(
                    $data['error'] ?? "Client error: {$statusCode}",
                    $statusCode,
                    null,
                    $data
                );
            }

            if ($statusCode >= 500) {
                throw new FlowCatalystException(
                    $data['error'] ?? "Server error: {$statusCode}",
                    $statusCode,
                    null,
                    $data
                );
            }

            return $data;
        } catch (RequestException $e) {
            throw new FlowCatalystException(
                'Request failed: ' . $e->getMessage(),
                $e->getCode(),
                $e
            );
        } catch (GuzzleException $e) {
            throw new FlowCatalystException(
                'HTTP client error: ' . $e->getMessage(),
                0,
                $e
            );
        }
    }

    /**
     * Get the base URL.
     */
    public function getBaseUrl(): string
    {
        return $this->baseUrl;
    }

    /**
     * Get the token manager.
     */
    public function getTokenManager(): OidcTokenManager
    {
        return $this->tokenManager;
    }
}
