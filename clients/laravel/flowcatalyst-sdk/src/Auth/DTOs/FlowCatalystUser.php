<?php

declare(strict_types=1);

namespace FlowCatalyst\Auth\DTOs;

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
         * The user's email address.
         */
        public string $email,

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
    ) {}

    /**
     * Get the list of client IDs this user has access to.
     * Returns ['*'] for platform-wide access, or specific client IDs.
     *
     * @return array<string>
     */
    public function getClients(): array
    {
        $clients = $this->claims['clients'] ?? [];
        return is_array($clients) ? $clients : [];
    }

    /**
     * Check if the user has platform-wide (anchor) access.
     */
    public function hasFullAccess(): bool
    {
        return in_array('*', $this->getClients(), true);
    }

    /**
     * Check if the user has access to a specific client.
     */
    public function hasClientAccess(string $clientId): bool
    {
        if ($this->hasFullAccess()) {
            return true;
        }
        return in_array($clientId, $this->getClients(), true);
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
            email: $claims['email'] ?? $claims['preferred_username'] ?? throw new \InvalidArgumentException('Missing email claim'),
            name: $claims['name'] ?? null,
            claims: $claims,
            accessToken: $accessToken,
            refreshToken: $refreshToken,
        );
    }
}
