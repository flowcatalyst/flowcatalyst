<?php

declare(strict_types=1);

namespace FlowCatalyst\Client\Resources;

use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\Client\Resources\Concerns\DeprecatedAdminAccess;
use FlowCatalyst\DTOs\Client;
use FlowCatalyst\DTOs\ClientApplication;
use FlowCatalyst\DTOs\Requests\CreateClientRequest;
use FlowCatalyst\DTOs\Requests\UpdateClientApplicationsRequest;
use FlowCatalyst\DTOs\Requests\UpdateClientRequest;
use FlowCatalyst\DTOs\Responses\ClientList;

class Clients
{
    use DeprecatedAdminAccess;

    private const ADMIN_ACCESSOR = 'clients';

    public function __construct(
        private readonly FlowCatalystClient $client
    ) {}

    /**
     * List clients.
     */
    public function list(?string $status = null): ClientList
    {
        $query = $status !== null
            ? '?' . http_build_query(['status' => $status])
            : '';
        $response = $this->client->request('GET', "/api/clients{$query}");

        return ClientList::fromArray($response);
    }

    /**
     * Search clients by name or identifier.
     *
     * @deprecated Use `$client->admin()->clients()->search()`; the notice fires once per process.
     */
    public function search(string $term): ClientList
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $query = '?' . http_build_query(['q' => $term]);
        $response = $this->client->request('GET', "/api/clients/search{$query}");

        return ClientList::fromArray($response);
    }

    /**
     * Get a client by ID.
     */
    public function get(string $id): Client
    {
        $response = $this->client->request('GET', "/api/clients/{$id}");

        return Client::fromArray($response);
    }

    /**
     * Get a client by its identifier (slug).
     */
    public function getByIdentifier(string $identifier): Client
    {
        $response = $this->client->request('GET', "/api/clients/by-identifier/{$identifier}");

        return Client::fromArray($response);
    }

    /**
     * Get a client by ID, returning the platform's decoded body unchanged.
     *
     * Unlike {@see get()}, nothing is defaulted: the {@see Client} DTO fills a
     * missing `status` with 'ACTIVE', which hides a malformed or partial
     * answer. Use this when you must see exactly what the platform said.
     *
     * @return array<string, mixed>
     */
    public function getRaw(string $id): array
    {
        return $this->client->request('GET', '/api/clients/' . rawurlencode($id));
    }

    /**
     * Get a client by identifier (slug), returning the platform's decoded
     * body unchanged. See {@see getRaw()} for why.
     *
     * @return array<string, mixed>
     */
    public function getByIdentifierRaw(string $identifier): array
    {
        return $this->client->request('GET', '/api/clients/by-identifier/' . rawurlencode($identifier));
    }

    /**
     * Create a new client.
     *
     * Returns the created client's ID. The platform's create endpoint
     * returns `{id}` only — use `get($id)` to fetch the full record.
     *
     * @deprecated Use `$client->admin()->clients()->create()`; the notice fires once per process.
     */
    public function create(CreateClientRequest $request): string
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $response = $this->client->request('POST', '/api/clients', [
            'json' => $request->toArray(),
        ]);

        return (string) $response['id'];
    }

    /**
     * Update a client.
     *
     * @deprecated Use `$client->admin()->clients()->update()`; the notice fires once per process.
     */
    public function update(string $id, UpdateClientRequest $request): Client
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $response = $this->client->request('PUT', "/api/clients/{$id}", [
            'json' => $request->toArray(),
        ]);

        return Client::fromArray($response);
    }

    /**
     * Activate a client.
     *
     * The platform responds with `{message}` only. Call `get($id)` if you
     * need the refreshed record.
     *
     * @deprecated Use `$client->admin()->clients()->activate()`; the notice fires once per process.
     */
    public function activate(string $id): void
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $this->client->request('POST', "/api/clients/{$id}/activate");
    }

    /**
     * Suspend a client. A reason is required.
     *
     * @deprecated Use `$client->admin()->clients()->suspend()`; the notice fires once per process.
     */
    public function suspend(string $id, string $reason): void
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $this->client->request('POST', "/api/clients/{$id}/suspend", [
            'json' => ['reason' => $reason],
        ]);
    }

    /**
     * Deactivate a client. A reason is required.
     *
     * @deprecated Use `$client->admin()->clients()->deactivate()`; the notice fires once per process.
     */
    public function deactivate(string $id, string $reason): void
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $this->client->request('POST', "/api/clients/{$id}/deactivate", [
            'json' => ['reason' => $reason],
        ]);
    }

    /**
     * Add a note to a client's audit history.
     *
     * @deprecated Use `$client->admin()->clients()->addNote()`; the notice fires once per process.
     */
    public function addNote(string $id, string $category, string $text): void
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $this->client->request('POST', "/api/clients/{$id}/notes", [
            'json' => [
                'category' => $category,
                'text' => $text,
            ],
        ]);
    }

    /**
     * List applications configured for a client.
     *
     * @return ClientApplication[]
     *
     * @deprecated Use `$client->admin()->clients()->getApplications()`; the notice fires once per process.
     */
    public function getApplications(string $id): array
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $response = $this->client->request('GET', "/api/clients/{$id}/applications");

        /** @var array<int, array<string, mixed>> $rows */
        $rows = $response['applications'] ?? [];
        return array_map(
            fn(array $row) => ClientApplication::fromArray($row),
            $rows,
        );
    }

    /**
     * Declaratively set the applications enabled for a client.
     *
     * @deprecated Use `$client->admin()->clients()->updateApplications()`; the notice fires once per process.
     */
    public function updateApplications(
        string $id,
        UpdateClientApplicationsRequest $request,
    ): void {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $this->client->request('PUT', "/api/clients/{$id}/applications", [
            'json' => $request->toArray(),
        ]);
    }

    /**
     * Enable a single application for a client.
     *
     * @deprecated Use `$client->admin()->clients()->enableApplication()`; the notice fires once per process.
     */
    public function enableApplication(string $clientId, string $applicationId): void
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $this->client->request('POST', "/api/clients/{$clientId}/applications/{$applicationId}/enable");
    }

    /**
     * Disable a single application for a client.
     *
     * @deprecated Use `$client->admin()->clients()->disableApplication()`; the notice fires once per process.
     */
    public function disableApplication(string $clientId, string $applicationId): void
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $this->client->request('POST', "/api/clients/{$clientId}/applications/{$applicationId}/disable");
    }
}
