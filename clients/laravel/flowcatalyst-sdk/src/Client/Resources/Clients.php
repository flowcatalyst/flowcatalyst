<?php

declare(strict_types=1);

namespace FlowCatalyst\Client\Resources;

use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\DTOs\Client;

class Clients
{
    public function __construct(
        private readonly FlowCatalystClient $client
    ) {}

    /**
     * List all clients.
     *
     * @return array{clients: Client[], total: int}
     */
    public function list(): array
    {
        $response = $this->client->request('GET', '/api/admin/clients');

        return [
            'clients' => array_map(
                fn(array $item) => Client::fromArray($item),
                $response['clients'] ?? []
            ),
            'total' => $response['total'] ?? 0,
        ];
    }

    /**
     * Get a client by ID.
     */
    public function get(string $id): Client
    {
        $response = $this->client->request('GET', "/api/admin/clients/{$id}");

        return Client::fromArray($response);
    }

    /**
     * Get a client by identifier.
     */
    public function getByIdentifier(string $identifier): Client
    {
        $response = $this->client->request('GET', "/api/admin/clients/by-identifier/{$identifier}");

        return Client::fromArray($response);
    }

    /**
     * Create a new client.
     *
     * @param array{
     *     name: string,
     *     identifier: string
     * } $data
     */
    public function create(array $data): Client
    {
        $response = $this->client->request('POST', '/api/admin/clients', [
            'json' => $data,
        ]);

        return Client::fromArray($response);
    }

    /**
     * Update a client.
     *
     * @param array{
     *     name?: string,
     *     description?: string
     * } $data
     */
    public function update(string $id, array $data): Client
    {
        $response = $this->client->request('PUT', "/api/admin/clients/{$id}", [
            'json' => $data,
        ]);

        return Client::fromArray($response);
    }

    /**
     * Activate a client.
     */
    public function activate(string $id): Client
    {
        $response = $this->client->request('POST', "/api/admin/clients/{$id}/activate");

        return Client::fromArray($response);
    }

    /**
     * Deactivate a client.
     */
    public function deactivate(string $id): Client
    {
        $response = $this->client->request('POST', "/api/admin/clients/{$id}/deactivate");

        return Client::fromArray($response);
    }
}
