<?php

declare(strict_types=1);

namespace FlowCatalyst\Attributes;

use Attribute;
use FlowCatalyst\DTOs\PermissionInput;

/**
 * Marks a class as a role definition for FlowCatalyst.
 *
 * Usage:
 * ```php
 * use FlowCatalyst\DTOs\PermissionInput;
 *
 * #[AsRole(
 *     name: 'admin',
 *     displayName: 'Administrator',
 *     description: 'Full administrative access',
 *     permissions: [
 *         new PermissionInput('myapp', 'users', 'user', 'view'),
 *         new PermissionInput('myapp', 'users', 'user', 'create'),
 *         new PermissionInput('myapp', 'settings', 'config', 'manage'),
 *     ],
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
     * Role name format: lowercase alphanumeric with hyphens/underscores, cannot start/end with hyphen or underscore.
     */
    private const NAME_PATTERN = '/^[a-z0-9]([a-z0-9_-]*[a-z0-9])?$/';

    /**
     * @param string $name Role name (will be prefixed with app code)
     * @param string|null $displayName Human-friendly display name
     * @param string|null $description Role description
     * @param PermissionInput[] $permissions Array of structured permissions
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
     * Validate the role name.
     *
     * @return string|null Error message if invalid, null if valid
     */
    public function validate(): ?string
    {
        if (empty($this->name)) {
            return 'Role name cannot be empty';
        }

        if (str_contains($this->name, ':')) {
            return "Role name cannot contain colons (the app code will be prefixed automatically): {$this->name}";
        }

        if (!preg_match(self::NAME_PATTERN, $this->name)) {
            return "Role name must be lowercase alphanumeric with hyphens/underscores (cannot start/end with hyphen or underscore): {$this->name}";
        }

        return null;
    }

    /**
     * Check if this role definition is valid.
     */
    public function isValid(): bool
    {
        return $this->validate() === null;
    }

    /**
     * Convert to array format for API sync.
     */
    public function toArray(): array
    {
        $data = [
            'name' => $this->name,
        ];

        if ($this->displayName !== null) {
            $data['displayName'] = $this->displayName;
        }

        if ($this->description !== null) {
            $data['description'] = $this->description;
        }

        if (!empty($this->permissions)) {
            $data['permissions'] = array_map(
                fn(PermissionInput $p) => $p->toArray(),
                $this->permissions
            );
        }

        if ($this->clientManaged) {
            $data['clientManaged'] = true;
        }

        return $data;
    }
}
