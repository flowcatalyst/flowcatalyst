<?php

declare(strict_types=1);

namespace FlowCatalyst\Sync;

/**
 * Represents a principal (user) definition for syncing to FlowCatalyst.
 *
 * Each principal is identified by email. The sync will create new users or
 * update existing ones, and assign the specified roles (prefixed with the
 * application code).
 *
 * Example:
 * ```php
 * $principal = PrincipalDefinition::make('john@example.com', 'John Doe')
 *     ->withRoles(['admin', 'editor']);
 * ```
 */
final class PrincipalDefinition
{
    /**
     * @param string $email User's email address (unique identifier)
     * @param string $name Display name
     * @param string[] $roles Role short names (will be prefixed with app code)
     * @param bool $active Whether the user should be active
     */
    public function __construct(
        public readonly string $email,
        public readonly string $name,
        public readonly array $roles = [],
        public readonly bool $active = true,
    ) {}

    /**
     * Create a new principal definition with fluent syntax.
     */
    public static function make(string $email, string $name): self
    {
        return new self($email, $name);
    }

    /**
     * Create a copy with different roles.
     *
     * @param string[] $roles Role short names (e.g., 'admin', 'transport-planner')
     */
    public function withRoles(array $roles): self
    {
        return new self(
            email: $this->email,
            name: $this->name,
            roles: $roles,
            active: $this->active,
        );
    }

    /**
     * Create a copy that is inactive.
     */
    public function inactive(): self
    {
        return new self(
            email: $this->email,
            name: $this->name,
            roles: $this->roles,
            active: false,
        );
    }

    /**
     * Convert to array for the sync API.
     *
     * @return array<string, mixed>
     */
    public function toArray(): array
    {
        $data = [
            'email' => $this->email,
            'name' => $this->name,
        ];

        if (!empty($this->roles)) {
            $data['roles'] = $this->roles;
        }

        if (!$this->active) {
            $data['active'] = false;
        }

        return $data;
    }

    /**
     * Create from array (e.g., from cached definitions).
     *
     * @param array<string, mixed> $data
     */
    public static function fromArray(array $data): self
    {
        return new self(
            email: $data['email'],
            name: $data['name'],
            roles: $data['roles'] ?? [],
            active: $data['active'] ?? true,
        );
    }
}
