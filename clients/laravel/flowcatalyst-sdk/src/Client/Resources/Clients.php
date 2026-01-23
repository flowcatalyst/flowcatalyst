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
     * @return array{items: Client[]}
     */
    public function list(): array
    {
        $response = $this->client->request('GET', '/api/clients');

        return [
            'items' => array_map(
                fn(array $item) => Client::fromArray($item),
                $response['items'] ?? []
            ),
        ];
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
     * Get a client by code.
     */
    public function getByCode(string $code): Client
    {
        $response = $this->client->request('GET', "/api/clients/code/{$code}");

        return Client::fromArray($response);
    }

    /**
     * Create a new client.
     *
     * @param array{
     *     code: string,
     *     name: string,
     *     description?: string
     * } $data
     */
    public function create(array $data): Client
    {
        $response = $this->client->request('POST', '/api/clients', [
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
        $response = $this->client->request('PUT', "/api/clients/{$id}", [
            'json' => $data,
        ]);

        return Client::fromArray($response);
    }

    /**
     * Activate a client.
     */
    public function activate(string $id): Client
    {
        $response = $this->client->request('POST', "/api/clients/{$id}/activate");

        return Client::fromArray($response);
    }

    /**
     * Deactivate a client.
     */
    public function deactivate(string $id): Client
    {
        $response = $this->client->request('POST', "/api/clients/{$id}/deactivate");

        return Client::fromArray($response);
    }
}
