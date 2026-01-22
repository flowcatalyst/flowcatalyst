<?php

declare(strict_types=1);

namespace FlowCatalyst\Sync;

/**
 * Represents a role definition for syncing to FlowCatalyst.
 *
 * Can be used directly or converted to array for the sync API.
 */
final class RoleDefinition
{
    /**
     * @param string $name Unique role identifier (e.g., 'admin', 'editor')
     * @param string|null $displayName Human-readable name
     * @param string|null $description Role description
     * @param string[] $permissions List of permission codes this role grants
     * @param bool $clientManaged Whether clients can assign this role to their users
     */
    public function __construct(
        public readonly string $name,
        public readonly ?string $displayName = null,
        public readonly ?string $description = null,
        public readonly array $permissions = [],
        public readonly bool $clientManaged = false,
    ) {}

    /**
     * Create a new role definition with fluent syntax.
     */
    public static function make(string $name): self
    {
        return new self($name);
    }

    /**
     * Create a copy with a different display name.
     */
    public function withDisplayName(string $displayName): self
    {
        return new self(
            name: $this->name,
            displayName: $displayName,
            description: $this->description,
            permissions: $this->permissions,
            clientManaged: $this->clientManaged,
        );
    }

    /**
     * Create a copy with a different description.
     */
    public function withDescription(string $description): self
    {
        return new self(
            name: $this->name,
            displayName: $this->displayName,
            description: $description,
            permissions: $this->permissions,
            clientManaged: $this->clientManaged,
        );
    }

    /**
     * Create a copy with different permissions.
     *
     * @param string[] $permissions
     */
    public function withPermissions(array $permissions): self
    {
        return new self(
            name: $this->name,
            displayName: $this->displayName,
            description: $this->description,
            permissions: $permissions,
            clientManaged: $this->clientManaged,
        );
    }

    /**
     * Create a copy that is client-managed.
     */
    public function clientManaged(bool $clientManaged = true): self
    {
        return new self(
            name: $this->name,
            displayName: $this->displayName,
            description: $this->description,
            permissions: $this->permissions,
            clientManaged: $clientManaged,
        );
    }

    /**
     * Convert to array for the sync API.
     *
     * @return array<string, mixed>
     */
    public function toArray(): array
    {
        $data = ['name' => $this->name];

        if ($this->displayName !== null) {
            $data['displayName'] = $this->displayName;
        }

        if ($this->description !== null) {
            $data['description'] = $this->description;
        }

        if (!empty($this->permissions)) {
            $data['permissions'] = $this->permissions;
        }

        if ($this->clientManaged) {
            $data['clientManaged'] = true;
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
            name: $data['name'],
            displayName: $data['displayName'] ?? null,
            description: $data['description'] ?? null,
            permissions: $data['permissions'] ?? [],
            clientManaged: $data['clientManaged'] ?? false,
        );
    }
}
