<?php

declare(strict_types=1);

namespace FlowCatalyst\DTOs;

use FlowCatalyst\Enums\SchemaType;
use FlowCatalyst\Enums\SpecVersionStatus;

class SpecVersion
{
    public function __construct(
        public readonly string $version,
        public readonly string $mimeType,
        public readonly SchemaType $schemaType,
        public readonly SpecVersionStatus $status,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            version: $data['version'],
            mimeType: $data['mimeType'] ?? 'application/json',
            schemaType: SchemaType::tryFrom($data['schemaType'] ?? 'JSON_SCHEMA') ?? SchemaType::JSON_SCHEMA,
            status: SpecVersionStatus::tryFrom($data['status'] ?? 'CURRENT') ?? SpecVersionStatus::CURRENT,
        );
    }

    public function toArray(): array
    {
        return [
            'version' => $this->version,
            'mimeType' => $this->mimeType,
            'schemaType' => $this->schemaType->value,
            'status' => $this->status->value,
        ];
    }

    /**
     * Get the major version number.
     */
    public function getMajorVersion(): int
    {
        $parts = explode('.', $this->version);
        return (int) ($parts[0] ?? 0);
    }

    /**
     * Get the minor version number.
     */
    public function getMinorVersion(): int
    {
        $parts = explode('.', $this->version);
        return (int) ($parts[1] ?? 0);
    }
}
