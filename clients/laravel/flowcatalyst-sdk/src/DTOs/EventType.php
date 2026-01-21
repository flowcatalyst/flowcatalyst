<?php

declare(strict_types=1);

namespace FlowCatalyst\DTOs;

use FlowCatalyst\Enums\EventTypeStatus;

class EventType
{
    /**
     * @param SpecVersion[] $specVersions
     */
    public function __construct(
        public readonly string $id,
        public readonly string $code,
        public readonly string $name,
        public readonly ?string $description,
        public readonly EventTypeStatus $status,
        public readonly ?string $application,
        public readonly ?string $subdomain,
        public readonly ?string $aggregate,
        public readonly ?string $event,
        public readonly array $specVersions,
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
            status: EventTypeStatus::tryFrom($data['status'] ?? 'CURRENT') ?? EventTypeStatus::CURRENT,
            application: $data['application'] ?? null,
            subdomain: $data['subdomain'] ?? null,
            aggregate: $data['aggregate'] ?? null,
            event: $data['event'] ?? null,
            specVersions: array_map(
                fn(array $sv) => SpecVersion::fromArray($sv),
                $data['specVersions'] ?? []
            ),
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
            'status' => $this->status->value,
            'application' => $this->application,
            'subdomain' => $this->subdomain,
            'aggregate' => $this->aggregate,
            'event' => $this->event,
            'specVersions' => array_map(fn(SpecVersion $sv) => $sv->toArray(), $this->specVersions),
            'createdAt' => $this->createdAt,
            'updatedAt' => $this->updatedAt,
        ];
    }
}
