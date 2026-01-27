<?php

declare(strict_types=1);

namespace FlowCatalyst\Client\Resources;

use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\DTOs\Principal;
use FlowCatalyst\Exceptions\FlowCatalystException;

class Principals
{
    public function __construct(
        private readonly FlowCatalystClient $client
    ) {}

    /**
     * List all principals with optional filters.
     *
     * @return array{principals: Principal[], total: int}
     */
    public function list(
        ?string $clientId = null,
        ?string $type = null,
        ?bool $active = null,
        ?string $email = null
    ): array {
        $queryParams = [];

        if ($clientId !== null) {
            $queryParams['clientId'] = $clientId;
        }
        if ($type !== null) {
            $queryParams['type'] = $type;
        }
        if ($active !== null) {
            $queryParams['active'] = $active ? 'true' : 'false';
        }
        if ($email !== null) {
            $queryParams['email'] = $email;
        }

        $query = !empty($queryParams) ? '?' . http_build_query($queryParams) : '';

        $response = $this->client->request('GET', "/api/sdk/principals{$query}");

        return [
            'principals' => array_map(
                fn(array $item) => Principal::fromArray($item),
                $response['principals'] ?? []
            ),
            'total' => $response['total'] ?? 0,
        ];
    }

    /**
     * Get a principal by ID.
     */
    public function get(string $id): Principal
    {
        $response = $this->client->request('GET', "/api/sdk/principals/{$id}");

        return Principal::fromArray($response);
    }

    /**
     * Find a principal by email address.
     *
     * Returns null if no principal exists with the given email.
     */
    public function findByEmail(string $email): ?Principal
    {
        $result = $this->list(email: $email);

        if ($result['total'] === 0) {
            return null;
        }

        return $result['principals'][0] ?? null;
    }

    /**
     * Create a new user principal.
     *
     * @param array{
     *     email: string,
     *     name: string,
     *     password?: string,
     *     clientId?: string
     * } $data
     */
    public function createUser(array $data): Principal
    {
        $response = $this->client->request('POST', '/api/sdk/principals/user', [
            'json' => $data,
        ]);

        return Principal::fromArray($response);
    }

    /**
     * Update a principal's details.
     *
     * @param array{
     *     name?: string
     * } $data
     */
    public function update(string $id, array $data): Principal
    {
        $response = $this->client->request('PUT', "/api/sdk/principals/{$id}", [
            'json' => $data,
        ]);

        return Principal::fromArray($response);
    }

    /**
     * Activate a principal.
     */
    public function activate(string $id): void
    {
        $this->client->request('POST', "/api/sdk/principals/{$id}/activate");
    }

    /**
     * Deactivate a principal.
     */
    public function deactivate(string $id): void
    {
        $this->client->request('POST', "/api/sdk/principals/{$id}/deactivate");
    }

    /**
     * Get roles assigned to a principal.
     *
     * @return array{roles: array<array{roleName: string, assignmentSource: string, assignedAt: string}>}
     */
    public function getRoles(string $id): array
    {
        return $this->client->request('GET', "/api/sdk/principals/{$id}/roles");
    }

    /**
     * Add a single role to a principal.
     *
     * @return array{roleName: string, assignmentSource: string, assignedAt: string}
     */
    public function assignRole(string $id, string $roleName): array
    {
        return $this->client->request('POST', "/api/sdk/principals/{$id}/roles/{$roleName}");
    }

    /**
     * Remove a role from a principal.
     */
    public function removeRole(string $id, string $roleName): void
    {
        $this->client->request('DELETE', "/api/sdk/principals/{$id}/roles/{$roleName}");
    }

    /**
     * Batch assign roles to a principal (declarative).
     *
     * This sets the complete list of roles for the principal.
     * Roles not in the list will be removed.
     *
     * @param string[] $roles Role names to assign
     * @return array{roles: array, added: string[], removed: string[]}
     */
    public function assignRoles(string $id, array $roles): array
    {
        return $this->client->request('PUT', "/api/sdk/principals/{$id}/roles", [
            'json' => ['roles' => $roles],
        ]);
    }

    /**
     * Get client access grants for a principal.
     *
     * @return array{grants: array<array{id: string, clientId: string, grantedAt: string, expiresAt: ?string}>}
     */
    public function getClientAccessGrants(string $id): array
    {
        return $this->client->request('GET', "/api/sdk/principals/{$id}/clients");
    }

    /**
     * Grant client access to a principal.
     *
     * @return array{id: string, clientId: string, grantedAt: string, expiresAt: ?string}
     */
    public function grantClientAccess(string $id, string $clientId): array
    {
        return $this->client->request('POST', "/api/sdk/principals/{$id}/clients/{$clientId}");
    }

    /**
     * Revoke client access from a principal.
     */
    public function revokeClientAccess(string $id, string $clientId): void
    {
        $this->client->request('DELETE', "/api/sdk/principals/{$id}/clients/{$clientId}");
    }
}
