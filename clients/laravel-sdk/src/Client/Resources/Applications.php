<?php

declare(strict_types=1);

namespace FlowCatalyst\Client\Resources;

use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\Client\Resources\Concerns\DeprecatedAdminAccess;
use FlowCatalyst\DTOs\Application;
use FlowCatalyst\DTOs\ApplicationRole;
use FlowCatalyst\DTOs\ClientConfig;
use FlowCatalyst\DTOs\Requests\ClientConfigRequest;
use FlowCatalyst\DTOs\Requests\CreateApplicationRequest;
use FlowCatalyst\DTOs\Requests\UpdateApplicationRequest;
use FlowCatalyst\DTOs\Responses\ApplicationList;
use FlowCatalyst\DTOs\Responses\ClientConfigList;
use FlowCatalyst\DTOs\ServiceAccount;

class Applications
{
    use DeprecatedAdminAccess;

    private const ADMIN_ACCESSOR = 'applications';

    public function __construct(
        private readonly FlowCatalystClient $client
    ) {}

    /**
     * List applications.
     *
     * @deprecated Use `$client->admin()->applications()->list()`; the notice fires once per process.
     */
    public function list(?bool $active = null): ApplicationList
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $query = $active !== null
            ? '?' . http_build_query(['active' => $active ? 'true' : 'false'])
            : '';

        $response = $this->client->request('GET', "/api/applications{$query}");

        return ApplicationList::fromArray($response);
    }

    /**
     * Get an application by ID.
     */
    public function get(string $id): Application
    {
        $response = $this->client->request('GET', "/api/applications/{$id}");

        return Application::fromArray($response);
    }

    /**
     * Get an application by code.
     */
    public function getByCode(string $code): Application
    {
        $response = $this->client->request('GET', "/api/applications/by-code/{$code}");

        return Application::fromArray($response);
    }

    /**
     * Create a new application.
     *
     * @deprecated Use `$client->admin()->applications()->create()`; the notice fires once per process.
     */
    public function create(CreateApplicationRequest $request): Application
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $response = $this->client->request('POST', '/api/applications', [
            'json' => $request->toArray(),
        ]);

        return Application::fromArray($response);
    }

    /**
     * Update an application.
     *
     * @deprecated Use `$client->admin()->applications()->update()`; the notice fires once per process.
     */
    public function update(string $id, UpdateApplicationRequest $request): Application
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $response = $this->client->request('PUT', "/api/applications/{$id}", [
            'json' => $request->toArray(),
        ]);

        return Application::fromArray($response);
    }

    /**
     * Delete (deactivate) an application.
     *
     * @deprecated Use `$client->admin()->applications()->delete()`; the notice fires once per process.
     */
    public function delete(string $id): void
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $this->client->request('DELETE', "/api/applications/{$id}");
    }

    /**
     * Activate an application.
     *
     * @deprecated Use `$client->admin()->applications()->activate()`; the notice fires once per process.
     */
    public function activate(string $id): Application
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $response = $this->client->request('POST', "/api/applications/{$id}/activate");

        return Application::fromArray($response);
    }

    /**
     * Deactivate an application.
     *
     * @deprecated Use `$client->admin()->applications()->deactivate()`; the notice fires once per process.
     */
    public function deactivate(string $id): Application
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $response = $this->client->request('POST', "/api/applications/{$id}/deactivate");

        return Application::fromArray($response);
    }

    /**
     * Provision a service account for an application.
     *
     * @deprecated Use `$client->admin()->applications()->provisionServiceAccount()`; the notice fires once per process.
     */
    public function provisionServiceAccount(string $id): ServiceAccount
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $response = $this->client->request(
            'POST',
            "/api/applications/{$id}/provision-service-account",
        );

        return ServiceAccount::fromArray($response);
    }

    /**
     * Get the service account attached to an application.
     *
     * @deprecated Use `$client->admin()->applications()->getServiceAccount()`; the notice fires once per process.
     */
    public function getServiceAccount(string $id): ServiceAccount
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $response = $this->client->request('GET', "/api/applications/{$id}/service-account");

        return ServiceAccount::fromArray($response);
    }

    /**
     * List roles defined for an application (by TSID).
     *
     * Mounted under `/by-id` server-side so the admin TSID lookup doesn't
     * collide with the SDK's `/{app_code}/roles/sync` route.
     *
     * @return ApplicationRole[]
     */
    public function listRoles(string $id): array
    {
        $response = $this->client->request('GET', "/api/applications/by-id/{$id}/roles");

        /** @var array<int, array<string, mixed>> $rows */
        $rows = is_array($response) ? $response : [];
        return array_map(static fn(array $row) => ApplicationRole::fromArray($row), $rows);
    }

    /**
     * List per-client configurations for an application.
     *
     * @deprecated Use `$client->admin()->applications()->listClients()`; the notice fires once per process.
     */
    public function listClients(string $id): ClientConfigList
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $response = $this->client->request('GET', "/api/applications/{$id}/clients");

        return ClientConfigList::fromArray($response);
    }

    /**
     * Update per-client config for an application.
     *
     * @deprecated Use `$client->admin()->applications()->updateClientConfig()`; the notice fires once per process.
     */
    public function updateClientConfig(
        string $id,
        string $clientId,
        ClientConfigRequest $request,
    ): ClientConfig {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $response = $this->client->request(
            'PUT',
            "/api/applications/{$id}/clients/{$clientId}",
            [
                'json' => $request->toArray(),
            ],
        );

        return ClientConfig::fromArray($response);
    }

    /**
     * Enable an application for a specific client.
     *
     * @deprecated Use `$client->admin()->applications()->enableForClient()`; the notice fires once per process.
     */
    public function enableForClient(string $id, string $clientId): ClientConfig
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $response = $this->client->request(
            'POST',
            "/api/applications/{$id}/clients/{$clientId}/enable",
        );

        return ClientConfig::fromArray($response);
    }

    /**
     * Disable an application for a specific client.
     *
     * @deprecated Use `$client->admin()->applications()->disableForClient()`; the notice fires once per process.
     */
    public function disableForClient(string $id, string $clientId): ClientConfig
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $response = $this->client->request(
            'POST',
            "/api/applications/{$id}/clients/{$clientId}/disable",
        );

        return ClientConfig::fromArray($response);
    }
}
