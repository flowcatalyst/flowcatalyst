<?php

declare(strict_types=1);

namespace FlowCatalyst\DTOs;

use JsonSerializable;

/**
 * Result of revealing a SECRET platform-config property
 * (GET /api/config/{app}/{section}/{property}/secret).
 *
 * `$value` is the plaintext secret. It is sensitive: do not log it. The DTO
 * redacts it in var_dump/print_r/var_export, JSON encoding, serialize() and
 * toArray(); only the explicit `->value` property (or revealedValue()) reads it.
 */
final class PlatformConfigSecret implements JsonSerializable
{
    public const REDACTED = '***';

    public function __construct(
        public readonly string $applicationCode,
        public readonly string $section,
        public readonly string $property,
        public readonly ?string $clientId,
        public readonly string $value,
        public readonly string $updatedAt,
    ) {}

    /** @param array<string, mixed> $data */
    public static function fromArray(array $data): self
    {
        return new self(
            applicationCode: (string) ($data['applicationCode'] ?? ''),
            section: (string) ($data['section'] ?? ''),
            property: (string) ($data['property'] ?? ''),
            clientId: isset($data['clientId']) ? (string) $data['clientId'] : null,
            value: (string) ($data['value'] ?? ''),
            updatedAt: (string) ($data['updatedAt'] ?? ''),
        );
    }

    /** Explicit accessor for the plaintext. */
    public function revealedValue(): string
    {
        return $this->value;
    }

    /** @return array<string, mixed> `value` is redacted. */
    public function toArray(): array
    {
        return [
            'applicationCode' => $this->applicationCode,
            'section' => $this->section,
            'property' => $this->property,
            'clientId' => $this->clientId,
            'value' => self::REDACTED,
            'updatedAt' => $this->updatedAt,
        ];
    }

    /** @return array<string, mixed> */
    public function jsonSerialize(): array
    {
        return $this->toArray();
    }

    /** @return array<string, mixed> */
    public function __debugInfo(): array
    {
        return $this->toArray();
    }

    /** @return array<string, mixed> */
    public function __serialize(): array
    {
        return $this->toArray();
    }
}
