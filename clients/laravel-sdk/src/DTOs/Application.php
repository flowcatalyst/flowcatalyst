<?php

declare(strict_types=1);

namespace FlowCatalyst\DTOs;

class Application
{
    public function __construct(
        public readonly string $id,
        public readonly string $code,
        public readonly string $name,
        public readonly ?string $description,
        public readonly ?string $defaultBaseUrl,
        public readonly ?string $iconUrl,
        public readonly bool $active,
        public readonly string $createdAt,
        public readonly string $updatedAt,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            id: $data['id'],
            code: $data['code'],
            name: $data['name'],
            description: $data['description'] ?? null,
            defaultBaseUrl: $data['defaultBaseUrl'] ?? null,
            iconUrl: $data['iconUrl'] ?? null,
            active: $data['active'] ?? true,
            createdAt: $data['createdAt'] ?? '',
            updatedAt: $data['updatedAt'] ?? '',
        );
    }

    public function toArray(): array
    {
        return [
            'id' => $this->id,
            'code' => $this->code,
            'name' => $this->name,
            'description' => $this->description,
            'defaultBaseUrl' => $this->defaultBaseUrl,
            'iconUrl' => $this->iconUrl,
            'active' => $this->active,
            'createdAt' => $this->createdAt,
            'updatedAt' => $this->updatedAt,
        ];
    }
}
