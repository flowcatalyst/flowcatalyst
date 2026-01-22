<?php

declare(strict_types=1);

namespace FlowCatalyst\Sync;

/**
 * Represents an event type definition for syncing to FlowCatalyst.
 *
 * Can be used directly or converted to array for the sync API.
 */
final class EventTypeDefinition
{
    /**
     * @param string $code Unique event type code (e.g., 'user.created')
     * @param string $name Human-readable name
     * @param string|null $description Event type description
     */
    public function __construct(
        public readonly string $code,
        public readonly string $name,
        public readonly ?string $description = null,
    ) {}

    /**
     * Create a new event type definition with fluent syntax.
     */
    public static function make(string $code, string $name): self
    {
        return new self($code, $name);
    }

    /**
     * Create a copy with a different description.
     */
    public function withDescription(string $description): self
    {
        return new self(
            code: $this->code,
            name: $this->name,
            description: $description,
        );
    }

    /**
     * Convert to array for the sync API.
     *
     * @return array<string, mixed>
     */
    public function toArray(): array
    {
        $data = [
            'code' => $this->code,
            'name' => $this->name,
        ];

        if ($this->description !== null) {
            $data['description'] = $this->description;
        }

        return $data;
    }

    /**
     * Create from array (e.g., from cached definitions).
     *
     * @param array<string, mixed> $data
     */
    public static function fromArray(array $data): self
    {
        return new self(
            code: $data['code'],
            name: $data['name'],
            description: $data['description'] ?? null,
        );
    }
}
