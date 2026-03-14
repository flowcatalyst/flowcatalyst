<?php

declare(strict_types=1);

namespace FlowCatalyst\DTOs;

class Permission
{
    public function __construct(
        public readonly string $permission,
        public readonly ?string $application,
        public readonly ?string $context,
        public readonly ?string $aggregate,
        public readonly ?string $action,
        public readonly ?string $description,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            permission: $data['permission'],
            application: $data['application'] ?? null,
            context: $data['context'] ?? null,
            aggregate: $data['aggregate'] ?? null,
            action: $data['action'] ?? null,
            description: $data['description'] ?? null,
        );
    }

    public function toArray(): array
    {
        return [
            'permission' => $this->permission,
            'application' => $this->application,
            'context' => $this->context,
            'aggregate' => $this->aggregate,
            'action' => $this->action,
            'description' => $this->description,
        ];
    }
}
