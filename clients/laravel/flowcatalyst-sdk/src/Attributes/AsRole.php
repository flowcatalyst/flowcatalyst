<?php

declare(strict_types=1);

namespace FlowCatalyst\Attributes;

use Attribute;

/**
 * Marks a class as a role definition for FlowCatalyst.
 *
 * Usage:
 * ```php
 * #[AsRole(
 *     name: 'admin',
 *     displayName: 'Administrator',
 *     description: 'Full administrative access',
 *     permissions: ['users:read', 'users:write', 'settings:manage'],
 *     clientManaged: false
 * )]
 * class AdminRole {}
 * ```
 *
 * The role name will be prefixed with your application code when synced.
 * For example, if your app code is "myapp", the role becomes "myapp:admin".
 */
#[Attribute(Attribute::TARGET_CLASS)]
final class AsRole
{
    /**
     * @param string $name Role name (will be prefixed with app code)
     * @param string|null $displayName Human-friendly display name
     * @param string|null $description Role description
     * @param string[] $permissions Array of permission strings
     * @param bool $clientManaged Whether clients can assign this role
     */
    public function __construct(
        public readonly string $name,
        public readonly ?string $displayName = null,
        public readonly ?string $description = null,
        public readonly array $permissions = [],
        public readonly bool $clientManaged = false,
    ) {}

    /**
     * Convert to array format for API sync.
     */
    public function toArray(): array
    {
        return array_filter([
            'name' => $this->name,
            'displayName' => $this->displayName,
            'description' => $this->description,
            'permissions' => $this->permissions,
            'clientManaged' => $this->clientManaged,
        ], fn($value) => $value !== null && $value !== []);
    }
}
