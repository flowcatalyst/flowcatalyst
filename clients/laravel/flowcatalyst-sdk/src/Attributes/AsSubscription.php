<?php

declare(strict_types=1);

namespace FlowCatalyst\Attributes;

use Attribute;
use FlowCatalyst\Enums\DispatchMode;

/**
 * Marks a class as a subscription definition for FlowCatalyst.
 *
 * Usage:
 * ```php
 * #[AsSubscription(
 *     code: 'order-processor',
 *     name: 'Order Processor',
 *     description: 'Processes new orders',
 *     eventTypes: ['myapp:orders:order:created', 'myapp:orders:order:updated'],
 *     target: 'https://myapp.com/webhooks/orders',
 *     queue: 'orders',
 *     dispatchPoolCode: 'default',
 *     mode: DispatchMode::BLOCK_ON_ERROR
 * )]
 * class OrderProcessorSubscription {}
 * ```
 */
#[Attribute(Attribute::TARGET_CLASS)]
final class AsSubscription
{
    /**
     * @param string $code Unique subscription code
     * @param string $name Human-friendly name
     * @param string $target Target URL for webhook delivery
     * @param string $queue Queue name for message routing
     * @param string $dispatchPoolCode Dispatch pool code for rate limiting
     * @param string|null $description Subscription description
     * @param string[] $eventTypes Array of event type codes to subscribe to
     * @param array<array{key: string, value: string}> $customConfig Custom configuration entries
     * @param int|null $maxAgeSeconds Maximum age in seconds for dispatch jobs
     * @param int|null $delaySeconds Delay before first dispatch attempt
     * @param int|null $sequence Sequence number for ordering
     * @param DispatchMode|string|null $mode Dispatch mode (IMMEDIATE or BLOCK_ON_ERROR)
     * @param int|null $timeoutSeconds Timeout for target response
     * @param int|null $maxRetries Maximum retry attempts
     * @param bool|null $dataOnly If true, only send event data (no envelope)
     */
    public function __construct(
        public readonly string $code,
        public readonly string $name,
        public readonly string $target,
        public readonly string $queue,
        public readonly string $dispatchPoolCode,
        public readonly ?string $description = null,
        public readonly array $eventTypes = [],
        public readonly array $customConfig = [],
        public readonly ?int $maxAgeSeconds = null,
        public readonly ?int $delaySeconds = null,
        public readonly ?int $sequence = null,
        public readonly DispatchMode|string|null $mode = null,
        public readonly ?int $timeoutSeconds = null,
        public readonly ?int $maxRetries = null,
        public readonly ?bool $dataOnly = null,
    ) {}

    /**
     * Convert to array format for API sync.
     */
    public function toArray(): array
    {
        $mode = $this->mode;
        if ($mode instanceof DispatchMode) {
            $mode = $mode->value;
        }

        // Convert event types to binding format
        $eventTypeBindings = array_map(
            fn(string $code) => ['eventTypeCode' => $code],
            $this->eventTypes
        );

        return array_filter([
            'code' => $this->code,
            'name' => $this->name,
            'description' => $this->description,
            'eventTypes' => $eventTypeBindings ?: null,
            'target' => $this->target,
            'queue' => $this->queue,
            'dispatchPoolCode' => $this->dispatchPoolCode,
            'customConfig' => $this->customConfig ?: null,
            'maxAgeSeconds' => $this->maxAgeSeconds,
            'delaySeconds' => $this->delaySeconds,
            'sequence' => $this->sequence,
            'mode' => $mode,
            'timeoutSeconds' => $this->timeoutSeconds,
            'maxRetries' => $this->maxRetries,
            'dataOnly' => $this->dataOnly,
        ], fn($value) => $value !== null);
    }
}
