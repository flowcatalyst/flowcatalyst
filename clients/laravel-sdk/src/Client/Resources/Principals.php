<?php

declare(strict_types=1);

namespace FlowCatalyst\Client\Resources;

use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\Client\Resources\Concerns\DeprecatedAdminAccess;
use FlowCatalyst\DTOs\ClientAccessGrant;
use FlowCatalyst\DTOs\Principal;
use FlowCatalyst\DTOs\Requests\CreateUserRequest;
use FlowCatalyst\DTOs\Requests\SyncPrincipalEntry;
use FlowCatalyst\DTOs\Requests\UpdatePrincipalRequest;
use FlowCatalyst\DTOs\Responses\BatchAssignRolesResult;
use FlowCatalyst\DTOs\Responses\PrincipalList;
use FlowCatalyst\DTOs\Responses\SyncResult;
use FlowCatalyst\DTOs\RoleAssignment;

class Principals
{
    use DeprecatedAdminAccess;

    private const ADMIN_ACCESSOR = 'principals';

    public function __construct(
        private readonly FlowCatalystClient $client
    ) {}

    /**
     * List principals with optional filters.
     *
     * `email` is an exact (case-insensitive) match. `q` is a name/email
     * substring search — use `email` when you know the address.
     *
     * @deprecated Use `$client->admin()->principals()->list()`; the notice fires once per process.
     */
    public function list(
        ?string $clientId = null,
        ?string $type = null,
        ?bool $active = null,
        ?string $email = null,
    ): PrincipalList {
        $this->deprecatedAdminMethod(__FUNCTION__);
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
        $response = $this->client->request('GET', "/api/principals{$query}");

        return PrincipalList::fromArray($response);
    }

    /**
     * Get a principal by ID.
     *
     * @deprecated Use `$client->admin()->principals()->get()`; the notice fires once per process.
     */
    public function get(string $id): Principal
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $response = $this->client->request('GET', "/api/principals/{$id}");

        return Principal::fromArray($response);
    }

    /**
     * Find a principal by email address.
     *
     * Returns null if no principal exists with the given email. We
     * defensively verify the row we return actually matches the requested
     * email — older platform builds silently ignored unknown query params
     * and returned an unfiltered list.
     *
     * @deprecated Use `$client->admin()->principals()->findByEmail()`; the notice fires once per process.
     */
    public function findByEmail(string $email): ?Principal
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        // Internal lookup: must not raise the separate list() notice.
        $result = $this->viaAdmin()->list(email: $email);

        $needle = strtolower($email);
        foreach ($result->principals as $principal) {
            if (strtolower($principal->email ?? '') === $needle) {
                return $principal;
            }
        }

        return null;
    }

    /**
     * Create a new user principal.
     *
     * `enforcePasswordComplexity` is accepted but not currently enforced
     * by the platform on this endpoint — a supplied password always runs
     * the full complexity policy.
     *
     * Set `sendInvitation: false` on `$request` when your application is
     * taking over the invitation email itself (suppresses both the
     * "set your password" invite and the "account created" welcome). Set
     * `returnInviteLink: true` to have the platform mint the 72-hour
     * set-password link and return it as `$principal->inviteLink` instead
     * of emailing it (passwordless INTERNAL users only; always suppresses
     * the platform's own invite email). The returned link is a live
     * bearer credential — never log it. Set `inviteRedirectUri` to send the
     * invitee to your application (any absolute http(s) URL) after they
     * set their password.
     *
     * @deprecated Use `$client->admin()->principals()->createUser()`; the notice fires once per process.
     */
    public function createUser(CreateUserRequest $request): Principal
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $response = $this->client->request('POST', '/api/principals/users', [
            'json' => $request->toArray(),
        ]);

        return Principal::fromArray($response);
    }

    /**
     * Reset a user's password (admin operation).
     *
     * Only valid for internal-auth users. Pass
     * `enforcePasswordComplexity: false` when the caller enforces its own
     * password policy.
     *
     * @deprecated Use `$client->admin()->principals()->resetPassword()`; the notice fires once per process.
     */
    public function resetPassword(
        string $id,
        string $newPassword,
        bool $enforcePasswordComplexity = true,
    ): void {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $this->client->request('POST', "/api/principals/{$id}/reset-password", [
            'json' => [
                'newPassword' => $newPassword,
                'enforcePasswordComplexity' => $enforcePasswordComplexity,
            ],
        ]);
    }

    /**
     * Update a principal's mutable fields.
     *
     * @deprecated Use `$client->admin()->principals()->update()`; the notice fires once per process.
     */
    public function update(string $id, UpdatePrincipalRequest $request): Principal
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $response = $this->client->request('PUT', "/api/principals/{$id}", [
            'json' => $request->toArray(),
        ]);

        return Principal::fromArray($response);
    }

    /**
     * Activate a principal.
     *
     * @deprecated Use `$client->admin()->principals()->activate()`; the notice fires once per process.
     */
    public function activate(string $id): void
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $this->client->request('POST', "/api/principals/{$id}/activate");
    }

    /**
     * Deactivate a principal.
     *
     * @deprecated Use `$client->admin()->principals()->deactivate()`; the notice fires once per process.
     */
    public function deactivate(string $id): void
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $this->client->request('POST', "/api/principals/{$id}/deactivate");
    }

    /**
     * Get roles assigned to a principal.
     *
     * @return RoleAssignment[]
     *
     * @deprecated Use `$client->admin()->principals()->getRoles()`; the notice fires once per process.
     */
    public function getRoles(string $id): array
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $response = $this->client->request('GET', "/api/principals/{$id}/roles");

        /** @var array<int, array<string, mixed>> $rows */
        $rows = $response['roles'] ?? [];
        return array_map(
            fn(array $row) => RoleAssignment::fromArray($row),
            $rows,
        );
    }

    /**
     * Add a single role to a principal (additive — keeps existing roles).
     *
     * Renamed from `assignRole` to make the additive-vs-replace distinction
     * visible at the call site (paired with `setRoles` for replace-all).
     * Returns the principal after the assignment.
     *
     * @deprecated Use `$client->admin()->principals()->addRole()`; the notice fires once per process.
     */
    public function addRole(string $id, string $roleName): Principal
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $response = $this->client->request('POST', "/api/principals/{$id}/roles", [
            'json' => ['role' => $roleName],
        ]);

        return Principal::fromArray($response);
    }

    /**
     * Remove a single role from a principal. Returns the principal after
     * the removal.
     *
     * @deprecated Use `$client->admin()->principals()->removeRole()`; the notice fires once per process.
     */
    public function removeRole(string $id, string $roleName): Principal
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $response = $this->client->request('DELETE', "/api/principals/{$id}/roles/{$roleName}");

        return Principal::fromArray($response);
    }

    /**
     * Replace all roles on a principal with the given set (declarative).
     *
     * Renamed from `assignRoles` so the replace semantics are obvious
     * (paired with `addRole` for additive). Roles not in `$roles` are
     * removed. Returns the full role state plus the diff that was applied.
     *
     * @param string[] $roles Role names to assign
     *
     * @deprecated Use `$client->admin()->principals()->setRoles()`; the notice fires once per process.
     */
    public function setRoles(string $id, array $roles): BatchAssignRolesResult
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $response = $this->client->request('PUT', "/api/principals/{$id}/roles", [
            'json' => ['roles' => $roles],
        ]);

        return BatchAssignRolesResult::fromArray($response);
    }

    /**
     * List client-access grants for a principal.
     *
     * @return ClientAccessGrant[]
     *
     * @deprecated Use `$client->admin()->principals()->getClientAccessGrants()`; the notice fires once per process.
     */
    public function getClientAccessGrants(string $id): array
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $response = $this->client->request('GET', "/api/principals/{$id}/client-access");

        /** @var array<int, array<string, mixed>> $rows */
        $rows = $response['grants'] ?? [];
        return array_map(
            fn(array $row) => ClientAccessGrant::fromArray($row),
            $rows,
        );
    }

    /**
     * Grant a principal access to a client.
     *
     * @deprecated Use `$client->admin()->principals()->grantClientAccess()`; the notice fires once per process.
     */
    public function grantClientAccess(string $id, string $clientId): ClientAccessGrant
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $response = $this->client->request('POST', "/api/principals/{$id}/client-access", [
            'json' => ['clientId' => $clientId],
        ]);

        return ClientAccessGrant::fromArray($response);
    }

    /**
     * Change a USER principal's scope and client association with explicit
     * intent. Anchor-only on the platform (others get 403).
     *
     * - `$clientId = '*'`: promote to ANCHOR (all-client access); `$mode` is ignored.
     * - a specific client id with `$mode = 'CHANGE_CLIENT'`: make it the home client.
     * - a specific client id with `$mode = 'TO_PARTNER'`: promote to PARTNER
     *   (previous home client plus this one).
     *
     * `$mode` is required by the platform for a specific client id (400
     * MODE_REQUIRED otherwise). Returns the updated principal.
     */
    public function setClientAssociation(string $id, string $clientId, ?string $mode = null): Principal
    {
        $body = ['clientId' => $clientId];
        if ($mode !== null) {
            $body['mode'] = $mode;
        }

        $response = $this->client->request('PUT', "/api/principals/{$id}/client-association", [
            'json' => $body,
        ]);

        return Principal::fromArray($response);
    }

    /**
     * Revoke a principal's access to a client.
     *
     * @deprecated Use `$client->admin()->principals()->revokeClientAccess()`; the notice fires once per process.
     */
    public function revokeClientAccess(string $id, string $clientId): void
    {
        $this->deprecatedAdminMethod(__FUNCTION__);
        $this->client->request('DELETE', "/api/principals/{$id}/client-access/{$clientId}");
    }

    /**
     * Sync principals for an application (declarative).
     *
     * Creates/updates user principals and assigns roles (prefixed with the
     * application code). When `$removeUnlisted` is true, SDK-synced roles
     * on unlisted principals are removed.
     *
     * @param SyncPrincipalEntry[] $principals
     */
    public function sync(
        string $appCode,
        array $principals,
        bool $removeUnlisted = false,
    ): SyncResult {
        $query = $removeUnlisted ? '?removeUnlisted=true' : '';

        $response = $this->client->request(
            'POST',
            "/api/applications/{$appCode}/principals/sync{$query}",
            [
                'json' => [
                    'principals' => array_map(
                        fn(SyncPrincipalEntry $entry) => $entry->toArray(),
                        $principals,
                    ),
                ],
            ],
        );

        return SyncResult::fromArray($response);
    }

    /**
     * Sync users platform-wide — declarative upsert keyed on email, with NO
     * application scope (`POST /api/principals/sync`).
     *
     * This is the application-less twin of {@see sync()}: use it when you are
     * "just syncing users" — migrating accounts and (via
     * {@see SyncPrincipalEntry::$passwordHash}) their existing password hashes —
     * without nesting the call under an application. Users are global (matched
     * by email), so an application code adds nothing here.
     *
     * Pure upsert: it never strips roles from unlisted users.
     *
     * @param SyncPrincipalEntry[] $users
     */
    public function syncUsers(array $users): SyncResult
    {
        $response = $this->client->request('POST', '/api/principals/sync', [
            'json' => [
                'principals' => array_map(
                    fn(SyncPrincipalEntry $entry) => $entry->toArray(),
                    $users,
                ),
            ],
        ]);

        return SyncResult::fromArray($response);
    }
}
