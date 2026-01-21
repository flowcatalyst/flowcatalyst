<?php

declare(strict_types=1);

namespace FlowCatalyst\Client\Resources;

use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\DTOs\Subscription;
use FlowCatalyst\Enums\DispatchMode;
use FlowCatalyst\Enums\SubscriptionSource;
use FlowCatalyst\Enums\SubscriptionStatus;

class Subscriptions
{
    public function __construct(
        private readonly FlowCatalystClient $client
    ) {}

    /**
     * List all subscriptions with optional filters.
     *
     * @param array $filters Optional filters: clientId, status
     * @return array{subscriptions: Subscription[], total: int}
     */
    public function list(array $filters = []): array
    {
        $query = http_build_query($filters);
        $endpoint = '/api/admin/platform/subscriptions' . ($query ? "?{$query}" : '');

        $response = $this->client->request('GET', $endpoint);

        return [
            'subscriptions' => array_map(
                fn(array $item) => Subscription::fromArray($item),
                $response['subscriptions'] ?? []
            ),
            'total' => $response['total'] ?? count($response['subscriptions'] ?? []),
        ];
    }

    /**
     * Get a subscription by ID.
     */
    public function get(string $id): Subscription
    {
        $response = $this->client->request('GET', "/api/admin/platform/subscriptions/{$id}");

        return Subscription::fromArray($response);
    }

    /**
     * Create a new subscription.
     *
     * @param array{
     *     code: string,
     *     name: string,
     *     description?: string,
     *     clientId?: string,
     *     eventTypes: array<array{eventTypeId?: string, eventTypeCode?: string, specVersion?: string}>,
     *     target: string,
     *     queue: string,
     *     dispatchPoolId: string,
     *     customConfig?: array<array{key: string, value: string}>,
     *     source?: SubscriptionSource|string,
     *     maxAgeSeconds?: int,
     *     delaySeconds?: int,
     *     sequence?: int,
     *     mode?: DispatchMode|string,
     *     timeoutSeconds?: int,
     *     dataOnly?: bool
     * } $data
     */
    public function create(array $data): Subscription
    {
        // Convert enums to strings
        if (isset($data['source']) && $data['source'] instanceof SubscriptionSource) {
            $data['source'] = $data['source']->value;
        }
        if (isset($data['mode']) && $data['mode'] instanceof DispatchMode) {
            $data['mode'] = $data['mode']->value;
        }

        // Default source to API when created via SDK
        $data['source'] ??= SubscriptionSource::API->value;

        $response = $this->client->request('POST', '/api/admin/platform/subscriptions', [
            'json' => $data,
        ]);

        return Subscription::fromArray($response);
    }

    /**
     * Update a subscription.
     *
     * @param array{
     *     name?: string,
     *     description?: string,
     *     eventTypes?: array<array{eventTypeId?: string, eventTypeCode?: string, specVersion?: string}>,
     *     target?: string,
     *     queue?: string,
     *     dispatchPoolId?: string,
     *     customConfig?: array<array{key: string, value: string}>,
     *     status?: SubscriptionStatus|string,
     *     maxAgeSeconds?: int,
     *     delaySeconds?: int,
     *     sequence?: int,
     *     mode?: DispatchMode|string,
     *     timeoutSeconds?: int,
     *     dataOnly?: bool
     * } $data
     */
    public function update(string $id, array $data): Subscription
    {
        // Convert enums to strings
        if (isset($data['status']) && $data['status'] instanceof SubscriptionStatus) {
            $data['status'] = $data['status']->value;
        }
        if (isset($data['mode']) && $data['mode'] instanceof DispatchMode) {
            $data['mode'] = $data['mode']->value;
        }

        $response = $this->client->request('PUT', "/api/admin/platform/subscriptions/{$id}", [
            'json' => $data,
        ]);

        return Subscription::fromArray($response);
    }

    /**
     * Delete a subscription.
     */
    public function delete(string $id): void
    {
        $this->client->request('DELETE', "/api/admin/platform/subscriptions/{$id}");
    }

    /**
     * Pause a subscription.
     */
    public function pause(string $id): array
    {
        return $this->client->request('POST', "/api/admin/platform/subscriptions/{$id}/pause");
    }

    /**
     * Resume a subscription.
     */
    public function resume(string $id): array
    {
        return $this->client->request('POST', "/api/admin/platform/subscriptions/{$id}/resume");
    }
}
