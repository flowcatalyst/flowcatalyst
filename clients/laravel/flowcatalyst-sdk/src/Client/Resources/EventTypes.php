<?php

declare(strict_types=1);

namespace FlowCatalyst\Client\Resources;

use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\DTOs\EventType;
use FlowCatalyst\Enums\SchemaType;

class EventTypes
{
    public function __construct(
        private readonly FlowCatalystClient $client
    ) {}

    /**
     * List all event types with optional filters.
     *
     * @param array $filters Optional filters: status, application, subdomain, aggregate
     * @return array{items: EventType[], total: int}
     */
    public function list(array $filters = []): array
    {
        $query = http_build_query($filters);
        $endpoint = '/api/event-types' . ($query ? "?{$query}" : '');

        $response = $this->client->request('GET', $endpoint);

        return [
            'items' => array_map(
                fn(array $item) => EventType::fromArray($item),
                $response['items'] ?? []
            ),
            'total' => count($response['items'] ?? []),
        ];
    }

    /**
     * Get an event type by ID.
     */
    public function get(string $id): EventType
    {
        $response = $this->client->request('GET', "/api/event-types/{$id}");

        return EventType::fromArray($response);
    }

    /**
     * Create a new event type.
     *
     * @param array{code: string, name: string, description?: string} $data
     */
    public function create(array $data): EventType
    {
        $response = $this->client->request('POST', '/api/event-types', [
            'json' => $data,
        ]);

        return EventType::fromArray($response);
    }

    /**
     * Update an event type.
     *
     * @param array{name?: string, description?: string} $data
     */
    public function update(string $id, array $data): EventType
    {
        $response = $this->client->request('PATCH', "/api/event-types/{$id}", [
            'json' => $data,
        ]);

        return EventType::fromArray($response);
    }

    /**
     * Add a schema version to an event type.
     *
     * @param array{version: string, mimeType: string, schema: string, schemaType: SchemaType|string} $schema
     */
    public function addSchema(string $id, array $schema): EventType
    {
        if (isset($schema['schemaType']) && $schema['schemaType'] instanceof SchemaType) {
            $schema['schemaType'] = $schema['schemaType']->value;
        }

        $response = $this->client->request('POST', "/api/event-types/{$id}/schemas", [
            'json' => $schema,
        ]);

        return EventType::fromArray($response);
    }

    /**
     * Finalise a schema version (FINALISING -> CURRENT).
     */
    public function finaliseSchema(string $id, string $version): EventType
    {
        $response = $this->client->request('POST', "/api/event-types/{$id}/schemas/{$version}/finalise");

        return EventType::fromArray($response);
    }

    /**
     * Deprecate a schema version (CURRENT -> DEPRECATED).
     */
    public function deprecateSchema(string $id, string $version): EventType
    {
        $response = $this->client->request('POST', "/api/event-types/{$id}/schemas/{$version}/deprecate");

        return EventType::fromArray($response);
    }

    /**
     * Archive an event type.
     */
    public function archive(string $id): EventType
    {
        $response = $this->client->request('POST', "/api/event-types/{$id}/archive");

        return EventType::fromArray($response);
    }

    /**
     * Delete an event type.
     */
    public function delete(string $id): void
    {
        $this->client->request('DELETE', "/api/event-types/{$id}");
    }

    /**
     * Get distinct application names for filtering.
     *
     * @return string[]
     */
    public function filterApplications(): array
    {
        $response = $this->client->request('GET', '/api/event-types/filters/applications');

        return $response['applications'] ?? $response ?? [];
    }

    /**
     * Get distinct subdomains for filtering.
     *
     * @return string[]
     */
    public function filterSubdomains(?string $application = null): array
    {
        $query = $application ? "?application={$application}" : '';
        $response = $this->client->request('GET', "/api/event-types/filters/subdomains{$query}");

        return $response['subdomains'] ?? $response ?? [];
    }

    /**
     * Get distinct aggregates for filtering.
     *
     * @return string[]
     */
    public function filterAggregates(?string $application = null, ?string $subdomain = null): array
    {
        $params = array_filter([
            'application' => $application,
            'subdomain' => $subdomain,
        ]);
        $query = $params ? '?' . http_build_query($params) : '';
        $response = $this->client->request('GET', "/api/event-types/filters/aggregates{$query}");

        return $response['aggregates'] ?? $response ?? [];
    }
}
