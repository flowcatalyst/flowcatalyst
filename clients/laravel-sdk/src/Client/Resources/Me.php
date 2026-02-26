<?php

declare(strict_types=1);

namespace FlowCatalyst\Client\Resources;

use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\DTOs\MyClient;
use FlowCatalyst\DTOs\MyApplication;

/**
 * Me Resource
 *
 * User-scoped endpoints for accessing resources the authenticated user has access to.
 * These endpoints do NOT require admin permissions.
 */
class Me
{
    public function __construct(
        private readonly FlowCatalystClient $client
    ) {}

    /**
     * Get clients the authenticated user has access to.
     *
     * Access is determined by user scope:
     * - ANCHOR: All active clients
     * - PARTNER: IDP granted clients + explicit grants
     * - CLIENT: Home client + IDP additional clients + explicit grants
     *
     * @return array{clients: MyClient[], total: int}
     */
    public function getClients(): array
    {
        $response = $this->client->request('GET', '/api/me/clients');

        return [
            'clients' => array_map(
                fn(array $item) => MyClient::fromArray($item),
                $response['clients'] ?? []
            ),
            'total' => $response['total'] ?? 0,
        ];
    }

    /**
     * Get a specific client the user has access to.
     */
    public function getClient(string $clientId): MyClient
    {
        $response = $this->client->request('GET', "/api/me/clients/{$clientId}");

        return MyClient::fromArray($response);
    }

    /**
     * Get applications enabled for a client the user has access to.
     *
     * @return array{applications: MyApplication[], total: int, clientId: string}
     */
    public function getClientApplications(string $clientId): array
    {
        $response = $this->client->request('GET', "/api/me/clients/{$clientId}/applications");

        return [
            'applications' => array_map(
                fn(array $item) => MyApplication::fromArray($item),
                $response['applications'] ?? []
            ),
            'total' => $response['total'] ?? 0,
            'clientId' => $response['clientId'] ?? $clientId,
        ];
    }
}
