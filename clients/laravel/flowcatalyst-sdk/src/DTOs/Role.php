<?php

declare(strict_types=1);

namespace FlowCatalyst\DTOs;

class Role
{
    /**
     * @param string[] $permissions
     */
    public function __construct(
        public readonly string $name,
        public readonly ?string $applicationCode,
        public readonly ?string $fullName,
        public readonly ?string $displayName,
        public readonly ?string $description,
        public readonly array $permissions,
        public readonly ?string $source,
        public readonly bool $clientManaged,
        public readonly ?string $createdAt,
        public readonly ?string $updatedAt,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            name: $data['name'],
            applicationCode: $data['applicationCode'] ?? null,
            fullName: $data['fullName'] ?? null,
            displayName: $data['displayName'] ?? null,
            description: $data['description'] ?? null,
            permissions: $data['permissions'] ?? [],
            source: $data['source'] ?? null,
            clientManaged: $data['clientManaged'] ?? false,
            createdAt: $data['createdAt'] ?? null,
            updatedAt: $data['updatedAt'] ?? null,
        );
    }

    public function toArray(): array
    {
        return [
            'name' => $this->name,
            'applicationCode' => $this->applicationCode,
            'fullName' => $this->fullName,
            'displayName' => $this->displayName,
            'description' => $this->description,
            'permissions' => $this->permissions,
            'source' => $this->source,
            'clientManaged' => $this->clientManaged,
            'createdAt' => $this->createdAt,
            'updatedAt' => $this->updatedAt,
        ];
    }

    /**
     * Check if this role is SDK-managed.
     */
    public function isSdkManaged(): bool
    {
        return $this->source === 'SDK';
    }

    /**
     * Check if this role is from code (hardcoded).
     */
    public function isFromCode(): bool
    {
        return $this->source === 'CODE';
    }
}
