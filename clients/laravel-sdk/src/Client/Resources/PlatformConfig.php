<?php

declare(strict_types=1);

namespace FlowCatalyst\Client\Resources;

use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\DTOs\PlatformConfigProperty;

/**
 * Platform configuration properties (`/api/config/...`,
 * `/api/platform-config/{app}`).
 *
 * Authorisation is enforced by the platform: anchors may read and write any
 * application's config; other callers need a platform-config access grant
 * (read, or write for `set`/`delete`) for the application, and see SECRET
 * values masked. Path segments are URL-encoded here.
 */
class PlatformConfig
{
    public const VALUE_TYPE_PLAIN = 'PLAIN';
    public const VALUE_TYPE_SECRET = 'SECRET';

    public function __construct(
        private readonly FlowCatalystClient $client
    ) {}

    /**
     * List all properties of an application.
     *
     * @return PlatformConfigProperty[]
     */
    public function list(string $application): array
    {
        $response = $this->listRaw($application);

        /** @var array<int, array<string, mixed>> $rows */
        $rows = $response['items'] ?? [];

        return array_map(
            fn(array $row) => PlatformConfigProperty::fromArray($row),
            $rows,
        );
    }

    /**
     * List properties and return the platform's decoded body unchanged
     * (`{items: [...]}`), for callers that validate the shape themselves.
     *
     * @return array<string, mixed>
     */
    public function listRaw(string $application): array
    {
        return $this->client->request('GET', '/api/platform-config/' . rawurlencode($application));
    }

    /**
     * Get one property. `$clientId` selects a client-scoped value; omit for
     * the global value. A missing property is a 404 FlowCatalystException.
     */
    public function get(
        string $application,
        string $section,
        string $property,
        ?string $clientId = null,
    ): PlatformConfigProperty {
        return PlatformConfigProperty::fromArray(
            $this->getRaw($application, $section, $property, $clientId)
        );
    }

    /**
     * Get one property as the platform's decoded body, unchanged.
     *
     * @return array<string, mixed>
     */
    public function getRaw(
        string $application,
        string $section,
        string $property,
        ?string $clientId = null,
    ): array {
        return $this->client->request(
            'GET',
            $this->path($application, $section, $property, $clientId),
        );
    }

    /**
     * Create or overwrite a property and return the stored record.
     *
     * @param string $valueType PLAIN or SECRET
     */
    public function set(
        string $application,
        string $section,
        string $property,
        string $value,
        string $valueType = self::VALUE_TYPE_PLAIN,
        ?string $description = null,
        ?string $clientId = null,
    ): PlatformConfigProperty {
        $body = ['value' => $value, 'valueType' => $valueType];
        if ($description !== null) {
            $body['description'] = $description;
        }

        $response = $this->client->request(
            'PUT',
            $this->path($application, $section, $property, $clientId),
            ['json' => $body],
        );

        return PlatformConfigProperty::fromArray($response);
    }

    /**
     * Delete a property. Idempotent on the platform: deleting a property that
     * does not exist succeeds (204).
     */
    public function delete(
        string $application,
        string $section,
        string $property,
        ?string $clientId = null,
    ): void {
        $this->client->request(
            'DELETE',
            $this->path($application, $section, $property, $clientId),
        );
    }

    private function path(string $application, string $section, string $property, ?string $clientId): string
    {
        $query = $clientId !== null ? '?' . http_build_query(['clientId' => $clientId]) : '';

        return sprintf(
            '/api/config/%s/%s/%s%s',
            rawurlencode($application),
            rawurlencode($section),
            rawurlencode($property),
            $query,
        );
    }
}
