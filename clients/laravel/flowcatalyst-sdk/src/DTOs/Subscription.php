<?php

declare(strict_types=1);

namespace FlowCatalyst\DTOs;

use FlowCatalyst\Enums\DispatchMode;
use FlowCatalyst\Enums\SubscriptionSource;
use FlowCatalyst\Enums\SubscriptionStatus;

class Subscription
{
    /**
     * @param EventTypeBinding[] $eventTypes
     * @param array<array{key: string, value: string}> $customConfig
     */
    public function __construct(
        public readonly string $id,
        public readonly string $code,
        public readonly string $name,
        public readonly ?string $description,
        public readonly ?string $clientId,
        public readonly ?string $clientIdentifier,
        public readonly array $eventTypes,
        public readonly string $target,
        public readonly string $queue,
        public readonly string $dispatchPoolId,
        public readonly ?string $dispatchPoolCode,
        public readonly SubscriptionStatus $status,
        public readonly SubscriptionSource $source,
        public readonly DispatchMode $mode,
        public readonly int $maxAgeSeconds,
        public readonly int $delaySeconds,
        public readonly int $sequence,
        public readonly int $timeoutSeconds,
        public readonly bool $dataOnly,
        public readonly array $customConfig,
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
            clientId: $data['clientId'] ?? null,
            clientIdentifier: $data['clientIdentifier'] ?? null,
            eventTypes: array_map(
                fn(array $et) => EventTypeBinding::fromArray($et),
                $data['eventTypes'] ?? []
            ),
            target: $data['target'],
            queue: $data['queue'],
            dispatchPoolId: $data['dispatchPoolId'],
            dispatchPoolCode: $data['dispatchPoolCode'] ?? null,
            status: SubscriptionStatus::tryFrom($data['status'] ?? 'ACTIVE') ?? SubscriptionStatus::ACTIVE,
            source: SubscriptionSource::tryFrom($data['source'] ?? 'API') ?? SubscriptionSource::API,
            mode: DispatchMode::tryFrom($data['mode'] ?? 'IMMEDIATE') ?? DispatchMode::IMMEDIATE,
            maxAgeSeconds: $data['maxAgeSeconds'] ?? 86400,
            delaySeconds: $data['delaySeconds'] ?? 0,
            sequence: $data['sequence'] ?? 99,
            timeoutSeconds: $data['timeoutSeconds'] ?? 30,
            dataOnly: $data['dataOnly'] ?? true,
            customConfig: $data['customConfig'] ?? [],
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
            'clientId' => $this->clientId,
            'clientIdentifier' => $this->clientIdentifier,
            'eventTypes' => array_map(fn(EventTypeBinding $et) => $et->toArray(), $this->eventTypes),
            'target' => $this->target,
            'queue' => $this->queue,
            'dispatchPoolId' => $this->dispatchPoolId,
            'dispatchPoolCode' => $this->dispatchPoolCode,
            'status' => $this->status->value,
            'source' => $this->source->value,
            'mode' => $this->mode->value,
            'maxAgeSeconds' => $this->maxAgeSeconds,
            'delaySeconds' => $this->delaySeconds,
            'sequence' => $this->sequence,
            'timeoutSeconds' => $this->timeoutSeconds,
            'dataOnly' => $this->dataOnly,
            'customConfig' => $this->customConfig,
            'createdAt' => $this->createdAt,
            'updatedAt' => $this->updatedAt,
        ];
    }
}
