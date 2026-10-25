<?php

declare(strict_types=1);

namespace FlowCatalyst\Client\Resources;

use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\DTOs\PlatformConfigProperty;
use FlowCatalyst\DTOs\PlatformConfigSecret;

/**
 * Platform configuration properties (`/api/config/...`,
 * `/api/platform-config/{app}`).
 *
 * Authorisation is enforced by the platform: anchors may read and write any
 * application's config; other callers need a platform-config access grant
 * (read, or write for `set`/`delete`) for the application. SECRET properties
 * always come back from `get`/`list` as the literal `***`, for every caller;
 * the only way to read a secret's plaintext is {@see revealSecret()}.
 * Path segments are URL-encoded here.
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
     * Get one property. SECRET properties return the literal `***` as `value`;
     * use {@see revealSecret()} for the plaintext. `$clientId` selects a client-scoped value; omit for
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
     * Reveal the plaintext of a SECRET property
     * (`GET /api/config/{app}/{section}/{property}/secret`).
     *
     * Requires a reveal permission: `platform:admin:config:reveal` (a
     * platform administrator), or `platform:application-service:config:reveal`
     * (an application's service account, which the platform confines to its
     * own applications and to the clients it is linked to: its explicit
     * clients, else the clients its application is enabled for, plus the
     * application-wide value). Every call is audited by the platform. Errors: 403 not authorised, 404 no such
     * property, 400 `NOT_A_SECRET` (the property is PLAIN; use `get`) - all
     * surfaced as the SDK's normal exceptions.
     *
     * The call is deliberately uncached and the response is never logged;
     * the returned DTO redacts `value` in var_dump/print_r/JSON/toArray()
     * output. Read it explicitly via `->value`. A caller wanting a cache
     * should add its own TTL cache around this call.
     */
    public function revealSecret(
        string $application,
        string $section,
        string $property,
        ?string $clientId = null,
    ): PlatformConfigSecret {
        return PlatformConfigSecret::fromArray(
            $this->client->request(
                'GET',
                $this->path($application, $section, $property, null, '/secret', $clientId),
            )
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

    private function path(string $application, string $section, string $property, ?string $clientId, string $suffix = '', ?string $clientIdOverride = null): string
    {
        $clientId = $clientIdOverride ?? $clientId;
        $query = $clientId !== null ? '?' . http_build_query(['clientId' => $clientId]) : '';

        return sprintf(
            '/api/config/%s/%s/%s%s%s',
            rawurlencode($application),
            rawurlencode($section),
            rawurlencode($property),
            $suffix,
            $query,
        );
    }
}
