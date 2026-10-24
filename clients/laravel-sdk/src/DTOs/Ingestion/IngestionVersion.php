<?php

declare(strict_types=1);

namespace FlowCatalyst\DTOs\Ingestion;

/**
 * One saved version of an ingestion. A saved version is not run until it is
 * live.
 */
final class IngestionVersion
{
    public function __construct(
        public readonly string $id,
        public readonly int $version,
        public readonly bool $live,
        public readonly IngestionDefinition $definition,
        public readonly string $createdAt = '',
        public readonly ?string $createdBy = null,
        public readonly ?string $appliedFrom = null,
    ) {}

    /**
     * @param array<string, mixed> $data
     */
    public static function fromArray(array $data): self
    {
        return new self(
            id: (string) $data['id'],
            version: (int) $data['version'],
            live: (bool) $data['live'],
            definition: IngestionDefinition::fromArray($data),
            createdAt: (string) ($data['createdAt'] ?? ''),
            createdBy: isset($data['createdBy']) ? (string) $data['createdBy'] : null,
            appliedFrom: isset($data['appliedFrom']) ? (string) $data['appliedFrom'] : null,
        );
    }
}
