<?php

declare(strict_types=1);

namespace FlowCatalyst\Definition;

/**
 * Container for scanned FlowCatalyst definitions.
 */
final class ScannedDefinitions
{
    /**
     * @param array<array<string, mixed>> $roles
     * @param array<array<string, mixed>> $eventTypes
     * @param array<array<string, mixed>> $subscriptions
     * @param array<array<string, mixed>> $dispatchPools
     */
    public function __construct(
        public readonly array $roles = [],
        public readonly array $eventTypes = [],
        public readonly array $subscriptions = [],
        public readonly array $dispatchPools = [],
    ) {}

    /**
     * Check if there are any definitions.
     */
    public function isEmpty(): bool
    {
        return empty($this->roles)
            && empty($this->eventTypes)
            && empty($this->subscriptions)
            && empty($this->dispatchPools);
    }

    /**
     * Get total count of all definitions.
     */
    public function count(): int
    {
        return count($this->roles)
            + count($this->eventTypes)
            + count($this->subscriptions)
            + count($this->dispatchPools);
    }

    /**
     * Convert to array (for JSON serialization).
     *
     * @return array<string, array<array<string, mixed>>>
     */
    public function toArray(): array
    {
        return [
            'roles' => $this->roles,
            'eventTypes' => $this->eventTypes,
            'subscriptions' => $this->subscriptions,
            'dispatchPools' => $this->dispatchPools,
        ];
    }

    /**
     * Create from array (for JSON deserialization).
     *
     * @param array<string, array<array<string, mixed>>> $data
     */
    public static function fromArray(array $data): self
    {
        return new self(
            roles: $data['roles'] ?? [],
            eventTypes: $data['eventTypes'] ?? [],
            subscriptions: $data['subscriptions'] ?? [],
            dispatchPools: $data['dispatchPools'] ?? [],
        );
    }
}
