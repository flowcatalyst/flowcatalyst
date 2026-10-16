<?php

declare(strict_types=1);

namespace FlowCatalyst\Client\Resources;

use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\Exceptions\FlowCatalystException;
use FlowCatalyst\DTOs\Requests\CreateRoleRequest;
use FlowCatalyst\DTOs\Requests\SyncRoleEntry;
use FlowCatalyst\DTOs\Requests\UpdateRoleRequest;
use FlowCatalyst\DTOs\Responses\RoleList;
use FlowCatalyst\DTOs\Responses\SyncResult;
use FlowCatalyst\DTOs\Role;

class Roles
{
    public function __construct(
        private readonly FlowCatalystClient $client
    ) {}

    /**
     * List roles.
     */
    public function list(
        ?string $applicationCode = null,
        ?string $source = null,
        ?bool $clientManaged = null,
    ): RoleList {
        $queryParams = [];
        if ($applicationCode !== null) {
            $queryParams['applicationCode'] = $applicationCode;
        }
        if ($source !== null) {
            $queryParams['source'] = $source;
        }
        if ($clientManaged !== null) {
            $queryParams['clientManaged'] = $clientManaged ? 'true' : 'false';
        }
        $query = !empty($queryParams) ? '?' . http_build_query($queryParams) : '';
        $response = $this->client->request('GET', "/api/roles{$query}");

        return RoleList::fromArray($response);
    }

    /**
     * Get a role by name or ID.
     */
    public function get(string $roleName): Role
    {
        $response = $this->client->request('GET', "/api/roles/{$roleName}");

        return Role::fromArray($response);
    }

    /**
     * Get a role by its full code.
     */
    public function getByCode(string $code): Role
    {
        $response = $this->client->request('GET', "/api/roles/by-code/{$code}");

        return Role::fromArray($response);
    }

    /**
     * Create a new role.
     *
     * Returns the created role's ID. Call `get($id)` if you need the
     * full record.
     */
    public function create(CreateRoleRequest $request): string
    {
        $response = $this->client->request('POST', '/api/roles', [
            'json' => $request->toArray(),
        ]);

        return (string) $response['id'];
    }

    /**
     * Update a role. The platform responds with 204 No Content.
     */
    public function update(string $roleName, UpdateRoleRequest $request): void
    {
        $this->client->request('PUT', "/api/roles/{$roleName}", [
            'json' => $request->toArray(),
        ]);
    }

    /**
     * Grant a permission to a role.
     */
    public function grantPermission(string $roleName, string $permission): Role
    {
        $response = $this->client->request('POST', "/api/roles/{$roleName}/permissions", [
            'json' => ['permission' => $permission],
        ]);

        return Role::fromArray($response);
    }

    /**
     * Revoke a permission from a role.
     */
    public function revokePermission(string $roleName, string $permission): Role
    {
        $response = $this->client->request('DELETE', "/api/roles/{$roleName}/permissions/{$permission}");

        return Role::fromArray($response);
    }

    /**
     * Delete a role.
     */
    public function delete(string $roleName): void
    {
        $this->client->request('DELETE', "/api/roles/{$roleName}");
    }

    /**
     * Sync roles for an application. Creates/updates roles with
     * source=`SDK` and, when `$removeUnlisted` is true, removes
     * SDK-managed roles not in the sync list.
     *
     * @param SyncRoleEntry[] $roles
     */
    public function sync(
        string $applicationCode,
        array $roles,
        bool $removeUnlisted = false,
    ): SyncResult {
        $query = $removeUnlisted ? '?removeUnlisted=true' : '';

        try {
            $response = $this->client->request(
                'POST',
                "/api/applications/{$applicationCode}/roles/sync{$query}",
                [
                    'json' => [
                        'roles' => array_map(
                            fn(SyncRoleEntry $entry) => $entry->toArray(),
                            $roles,
                        ),
                    ],
                ],
            );
        } catch (FlowCatalystException $e) {
            throw self::explainSyncFailure($e, array_values($roles));
        }

        return SyncResult::fromArray($response);
    }

    /**
     * The platform applies a roles sync atomically and reports only
     * "... failed at index N" (plus its machine code and any details). Name
     * the role at that index and carry the platform's code and details, so a
     * caller can tell which role failed and why. The original message is kept
     * verbatim as the prefix; the exception code stays the HTTP status, and
     * the original is chained as `previous`. Only a plain
     * FlowCatalystException is re-wrapped; subclasses (validation,
     * authentication) pass through untouched.
     *
     * @param SyncRoleEntry[] $roles
     */
    private static function explainSyncFailure(FlowCatalystException $e, array $roles): \Throwable
    {
        if (get_class($e) !== FlowCatalystException::class) {
            return $e;
        }

        $context = $e->getContext() ?? [];
        $message = $e->getMessage();
        $extra = [];

        if (preg_match('/at index (\d+)/', $message, $m) === 1) {
            $index = (int) $m[1];
            $context['failedIndex'] = $index;
            if (isset($roles[$index])) {
                $context['failedRole'] = $roles[$index]->name;
                $extra[] = sprintf("role '%s' (index %d)", $roles[$index]->name, $index);
            } else {
                $extra[] = sprintf('index %d', $index);
            }
        }

        $reason = $context['error'] ?? null;
        if (is_string($reason) && $reason !== '') {
            $extra[] = 'code ' . $reason;
        }
        if (isset($context['details']) && $context['details'] !== []) {
            $extra[] = 'details ' . json_encode($context['details']);
        }

        if ($extra === []) {
            return $e;
        }

        return new FlowCatalystException(
            $message . ' [' . implode('; ', $extra) . ']',
            $e->getCode(),
            $e,
            $context,
        );
    }
}
