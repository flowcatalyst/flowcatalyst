<?php

declare(strict_types=1);

namespace FlowCatalyst\Auth\DTOs;

use FlowCatalyst\Auth\Rbac\RbacCatalogue;

/**
 * Data transfer object representing an authenticated FlowCatalyst user.
 *
 * This DTO is passed to your handleAuthenticatedUser() method after successful
 * OIDC authentication. Use these claims to create or update your local user.
 *
 * @example
 * ```php
 * public function handleAuthenticatedUser(FlowCatalystUser $fcUser): mixed
 * {
 *     $user = User::updateOrCreate(
 *         ['email' => $fcUser->email],
 *         [
 *             'name' => $fcUser->name,
 *             'flowcatalyst_id' => $fcUser->sub,
 *             'clients' => $fcUser->clients,
 *         ]
 *     );
 *
 *     Auth::login($user);
 *     return $user;
 * }
 * ```
 */
final readonly class FlowCatalystUser
{
    public function __construct(
        /**
         * The user's unique identifier in FlowCatalyst (subject claim).
         * This is stable across sessions and should be used as the foreign key.
         */
        public string $sub,

        /**
         * The user's email address. `null` for SERVICE principals (client
         * credentials) and any caller without a user identity.
         */
        public ?string $email,

        /**
         * The user's display name (may be null).
         */
        public ?string $name,

        /**
         * The raw ID token claims from FlowCatalyst.
         * Contains additional claims like 'clients', 'roles', etc.
         * @var array<string, mixed>
         */
        public array $claims,

        /**
         * The access token for making API calls on behalf of the user.
         * Only set if you need to make user-scoped API calls.
         */
        public ?string $accessToken = null,

        /**
         * The refresh token for renewing the access token.
         */
        public ?string $refreshToken = null,

        /**
         * Resolved permissions (set by {@see withRbac()}). Empty when no
         * catalogue has been applied.
         *
         * @var array<int, string>
         */
        public array $permissions = [],

        /**
         * How the user authenticated for the current request:
         * `'session'` for browser flow, `'bearer'` for API callers.
         */
        public ?string $mechanism = null,
    ) {}

    /**
     * Get the raw clients claim entries.
     * Format is "id:identifier" or "*" for full access.
     *
     * @return array<string>
     */
    public function getClientsRaw(): array
    {
        $clients = $this->claims['clients'] ?? [];
        return is_array($clients) ? $clients : [];
    }

    /**
     * Get the list of client IDs this user has access to.
     * Returns ['*'] for platform-wide access, or specific client IDs.
     *
     * @return array<string>
     */
    public function getClientIds(): array
    {
        $raw = $this->getClientsRaw();
        return array_map(function (string $entry): string {
            if ($entry === '*') {
                return '*';
            }
            // Format is "id:identifier" - extract the ID part
            $parts = explode(':', $entry, 2);
            return $parts[0];
        }, $raw);
    }

    /**
     * Get the list of client identifiers this user has access to.
     * Returns ['*'] for platform-wide access, or specific client identifiers.
     *
     * @return array<string>
     */
    public function getClientIdentifiers(): array
    {
        $raw = $this->getClientsRaw();
        return array_map(function (string $entry): string {
            if ($entry === '*') {
                return '*';
            }
            // Format is "id:identifier" - extract the identifier part
            $parts = explode(':', $entry, 2);
            return $parts[1] ?? $parts[0]; // Fallback to ID if no identifier
        }, $raw);
    }

    /**
     * Get clients as associative array mapping ID to identifier.
     * Returns ['*' => '*'] for platform-wide access.
     *
     * @return array<string, string>
     */
    public function getClients(): array
    {
        $raw = $this->getClientsRaw();
        $result = [];
        foreach ($raw as $entry) {
            if ($entry === '*') {
                $result['*'] = '*';
            } else {
                $parts = explode(':', $entry, 2);
                $id = $parts[0];
                $identifier = $parts[1] ?? $parts[0];
                $result[$id] = $identifier;
            }
        }
        return $result;
    }

    /**
     * Check if the user has platform-wide (anchor) access.
     */
    public function hasFullAccess(): bool
    {
        return in_array('*', $this->getClientsRaw(), true);
    }

    /**
     * Check if the user has access to a specific client by ID.
     */
    public function hasClientAccess(string $clientId): bool
    {
        if ($this->hasFullAccess()) {
            return true;
        }
        return in_array($clientId, $this->getClientIds(), true);
    }

    /**
     * Check if the user has access to a specific client by identifier.
     */
    public function hasClientAccessByIdentifier(string $identifier): bool
    {
        if ($this->hasFullAccess()) {
            return true;
        }
        return in_array($identifier, $this->getClientIdentifiers(), true);
    }

    /**
     * Get user roles from the token claims.
     *
     * @return array<string>
     */
    public function getRoles(): array
    {
        $roles = $this->claims['roles'] ?? [];
        return is_array($roles) ? $roles : [];
    }

    /**
     * Check if the user has a specific role.
     */
    public function hasRole(string $role): bool
    {
        return in_array($role, $this->getRoles(), true);
    }

    /**
     * ALL — every role in `$roles` must be present.
     *
     * @param array<int, string> $roles
     */
    public function hasRoles(array $roles): bool
    {
        $owned = $this->getRoles();
        foreach ($roles as $r) {
            if (!in_array($r, $owned, true)) {
                return false;
            }
        }
        return true;
    }

    /**
     * ANY — at least one role in `$roles` must be present.
     *
     * @param array<int, string> $roles
     */
    public function hasAnyRole(array $roles): bool
    {
        $owned = $this->getRoles();
        foreach ($roles as $r) {
            if (in_array($r, $owned, true)) {
                return true;
            }
        }
        return false;
    }

    /**
     * ALL — every permission must be granted (wildcards honoured).
     *
     * @param array<int, string> $permissions
     */
    public function hasPermissionTo(array $permissions): bool
    {
        foreach ($permissions as $p) {
            if (!RbacCatalogue::matches($this->permissions, $p)) {
                return false;
            }
        }
        return true;
    }

    /**
     * ANY — at least one permission must be granted.
     *
     * @param array<int, string> $permissions
     */
    public function hasAnyPermissionTo(array $permissions): bool
    {
        foreach ($permissions as $p) {
            if (RbacCatalogue::matches($this->permissions, $p)) {
                return true;
            }
        }
        return false;
    }

    /**
     * User scope (`'anchor' | 'partner' | 'client'`) — lower-cased from
     * the `scope` claim. Returns `null` if the claim is missing.
     */
    public function getScope(): ?string
    {
        $scope = $this->claims['scope'] ?? null;
        return is_string($scope) ? strtolower($scope) : null;
    }

    /**
     * Whether this user has anchor (full platform) access. Anchor users
     * have `scope == 'anchor'` and/or `clients` containing `'*'`.
     */
    public function isAnchor(): bool
    {
        return $this->getScope() === 'anchor' || $this->hasFullAccess();
    }

    /**
     * Application codes derived from roles (the `applications` claim).
     *
     * @return array<int, string>
     */
    public function getApplications(): array
    {
        $apps = $this->claims['applications'] ?? [];
        return is_array($apps) ? array_values(array_filter($apps, 'is_string')) : [];
    }

    /**
     * Whether the user accessed via session cookie.
     */
    public function isSession(): bool
    {
        return $this->mechanism === 'session';
    }

    /**
     * Whether the user accessed via Bearer token.
     */
    public function isBearer(): bool
    {
        return $this->mechanism === 'bearer';
    }

    /**
     * Return a new principal with permissions resolved through `$catalogue`.
     * The original is left unchanged.
     */
    public function withRbac(RbacCatalogue $catalogue): self
    {
        return new self(
            sub: $this->sub,
            email: $this->email,
            name: $this->name,
            claims: $this->claims,
            accessToken: $this->accessToken,
            refreshToken: $this->refreshToken,
            permissions: $catalogue->resolve($this->getRoles()),
            mechanism: $this->mechanism,
        );
    }

    /**
     * Return a copy with a different `$mechanism` field. Used by the
     * middleware to tag Bearer-vs-session principals consistently.
     */
    public function withMechanism(string $mechanism): self
    {
        return new self(
            sub: $this->sub,
            email: $this->email,
            name: $this->name,
            claims: $this->claims,
            accessToken: $this->accessToken,
            refreshToken: $this->refreshToken,
            permissions: $this->permissions,
            mechanism: $mechanism,
        );
    }

    /**
     * Create from decoded JWT claims.
     *
     * @param array<string, mixed> $claims
     */
    public static function fromClaims(
        array $claims,
        ?string $accessToken = null,
        ?string $refreshToken = null
    ): self {
        return new self(
            sub: $claims['sub'] ?? throw new \InvalidArgumentException('Missing sub claim'),
            email: $claims['email'] ?? $claims['preferred_username'] ?? null,
            name: $claims['name'] ?? null,
            claims: $claims,
            accessToken: $accessToken,
            refreshToken: $refreshToken,
        );
    }

    /**
     * Build from FlowCatalyst access-token claims (both `authorization_code`
     * and `client_credentials` grants). Unlike {@see fromClaims()} this does
     * not require an `email` claim — SERVICE tokens never carry one.
     *
     * @param array<string, mixed> $claims
     */
    public static function fromAccessTokenClaims(
        array $claims,
        ?string $accessToken = null,
        ?string $refreshToken = null,
        ?string $mechanism = null,
    ): self {
        return new self(
            sub: $claims['sub'] ?? throw new \InvalidArgumentException('Missing sub claim'),
            email: isset($claims['email']) && is_string($claims['email']) ? $claims['email'] : null,
            name: isset($claims['name']) && is_string($claims['name']) ? $claims['name'] : null,
            claims: $claims,
            accessToken: $accessToken,
            refreshToken: $refreshToken,
            permissions: [],
            mechanism: $mechanism,
        );
    }
}
