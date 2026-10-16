<?php

declare(strict_types=1);

namespace FlowCatalyst\DTOs;

/**
 * One platform-config property as returned by
 * GET/PUT /api/config/{app}/{section}/{property} and by the `items` of
 * GET /api/platform-config/{app}.
 *
 * Secret values are masked (`***`) by the platform for non-anchor callers.
 */
class PlatformConfigProperty
{
    public function __construct(
        public readonly string $id,
        public readonly string $applicationCode,
        public readonly string $section,
        public readonly string $property,
        public readonly string $scope,
        public readonly string $valueType,
        public readonly string $value,
        public readonly ?string $clientId,
        public readonly ?string $description,
        public readonly string $createdAt,
        public readonly string $updatedAt,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            id: (string) ($data['id'] ?? ''),
            applicationCode: (string) ($data['applicationCode'] ?? ''),
            section: (string) ($data['section'] ?? ''),
            property: (string) ($data['property'] ?? ''),
            scope: (string) ($data['scope'] ?? ''),
            valueType: (string) ($data['valueType'] ?? ''),
            value: (string) ($data['value'] ?? ''),
            clientId: isset($data['clientId']) ? (string) $data['clientId'] : null,
            description: isset($data['description']) ? (string) $data['description'] : null,
            createdAt: (string) ($data['createdAt'] ?? ''),
            updatedAt: (string) ($data['updatedAt'] ?? ''),
        );
    }

    public function toArray(): array
    {
        return [
            'id' => $this->id,
            'applicationCode' => $this->applicationCode,
            'section' => $this->section,
            'property' => $this->property,
            'scope' => $this->scope,
            'valueType' => $this->valueType,
            'value' => $this->value,
            'clientId' => $this->clientId,
            'description' => $this->description,
            'createdAt' => $this->createdAt,
            'updatedAt' => $this->updatedAt,
        ];
    }
}
