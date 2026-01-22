<?php

declare(strict_types=1);

namespace FlowCatalyst\Attributes;

use Attribute;

/**
 * Marks a class as an event type definition for FlowCatalyst.
 *
 * Usage:
 * ```php
 * #[AsEventType(
 *     code: 'orders:order:created',
 *     name: 'Order Created',
 *     description: 'Emitted when a new order is placed'
 * )]
 * class OrderCreatedEvent {}
 * ```
 *
 * The code will be prefixed with your application code when synced.
 * For example, if your app code is "myapp", the event type becomes "myapp:orders:order:created".
 *
 * Code format recommendation: {subdomain}:{aggregate}:{event}
 */
#[Attribute(Attribute::TARGET_CLASS)]
final class AsEventType
{
    /**
     * @param string $code Event type code (will be prefixed with app code)
     * @param string $name Human-friendly name
     * @param string|null $description Event type description
     */
    public function __construct(
        public readonly string $code,
        public readonly string $name,
        public readonly ?string $description = null,
    ) {}

    /**
     * Convert to array format for API sync.
     */
    public function toArray(): array
    {
        return array_filter([
            'code' => $this->code,
            'name' => $this->name,
            'description' => $this->description,
        ], fn($value) => $value !== null);
    }
}
