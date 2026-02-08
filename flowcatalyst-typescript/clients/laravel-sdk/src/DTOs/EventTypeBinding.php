<?php

declare(strict_types=1);

namespace FlowCatalyst\DTOs;

class EventTypeBinding
{
    public function __construct(
        public readonly ?string $eventTypeId,
        public readonly ?string $eventTypeCode,
        public readonly ?string $specVersion,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            eventTypeId: $data['eventTypeId'] ?? null,
            eventTypeCode: $data['eventTypeCode'] ?? null,
            specVersion: $data['specVersion'] ?? null,
        );
    }

    public function toArray(): array
    {
        return array_filter([
            'eventTypeId' => $this->eventTypeId,
            'eventTypeCode' => $this->eventTypeCode,
            'specVersion' => $this->specVersion,
        ], fn($v) => $v !== null);
    }

    /**
     * Create a binding by event type ID.
     */
    public static function byId(string $eventTypeId, ?string $specVersion = null): self
    {
        return new self(
            eventTypeId: $eventTypeId,
            eventTypeCode: null,
            specVersion: $specVersion,
        );
    }

    /**
     * Create a binding by event type code.
     */
    public static function byCode(string $eventTypeCode, ?string $specVersion = null): self
    {
        return new self(
            eventTypeId: null,
            eventTypeCode: $eventTypeCode,
            specVersion: $specVersion,
        );
    }
}
