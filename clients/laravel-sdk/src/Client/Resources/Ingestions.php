<?php

declare(strict_types=1);

namespace FlowCatalyst\Client\Resources;

use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\DTOs\Ingestion\Ingestion;
use FlowCatalyst\DTOs\Ingestion\IngestionDefinition;
use FlowCatalyst\DTOs\Ingestion\IngestionList;
use FlowCatalyst\DTOs\Ingestion\IngestionVersion;

/**
 * Ingestions (`/api/ingestions...`), the source-kind catalogue and
 * "test connection" (`/api/ingestion-tests`). Sets are on {@see IngestionSets}.
 *
 * An ingestion belongs to an INTEGRATION application (by code) and a client
 * (id or identifier). Secrets in a definition are NAMES; this SDK never
 * handles a secret value for ingestions. Authorisation is enforced by the
 * platform. Errors surface as the SDK's normal exceptions: 400
 * (`INGESTION_TARGET_INVALID` and other validation) and 409
 * (`INGESTION_CODE_EXISTS`, a stale `expectedVersion`) as
 * FlowCatalystException with the platform's error code in `getContext()['error']`.
 * Path segments are URL-encoded here.
 */
class Ingestions
{
    public function __construct(
        private readonly FlowCatalystClient $client
    ) {}

    /**
     * List ingestions, a page at a time. `$clientId` is an id or identifier;
     * `$status` is ACTIVE or PAUSED. Continue with `$after = $page->nextCursor`.
     */
    public function list(
        ?string $applicationCode = null,
        ?string $clientId = null,
        ?string $q = null,
        ?string $status = null,
        ?string $kind = null,
        ?string $after = null,
        ?int $limit = null,
    ): IngestionList {
        $query = $this->query([
            'applicationCode' => $applicationCode,
            'clientId' => $clientId,
            'q' => $q,
            'status' => $status,
            'kind' => $kind,
            'after' => $after,
            'limit' => $limit,
        ]);

        return IngestionList::fromArray($this->client->request('GET', "/api/ingestions{$query}"));
    }

    /**
     * Create an ingestion. `$live` (platform default true) false saves the
     * first version without making it live.
     */
    public function create(
        string $application,
        string $client,
        string $code,
        IngestionDefinition $definition,
        ?bool $live = null,
    ): Ingestion {
        $body = ['application' => $application, 'client' => $client, 'code' => $code]
            + $definition->toArray();
        if ($live !== null) {
            $body['live'] = $live;
        }

        return Ingestion::fromArray(
            $this->client->request('POST', '/api/ingestions', ['json' => $body])
        );
    }

    public function get(string $id): Ingestion
    {
        return Ingestion::fromArray($this->client->request('GET', $this->path($id)));
    }

    /**
     * Replace the definition as the next live version. `$expectedVersion` is
     * the live version the caller read (`Ingestion::$version`, 0 when none is
     * live); a different current version is a 409.
     */
    public function update(string $id, IngestionDefinition $definition, int $expectedVersion): Ingestion
    {
        return Ingestion::fromArray($this->client->request(
            'PUT',
            $this->path($id),
            ['json' => ['expectedVersion' => $expectedVersion] + $definition->toArray()],
        ));
    }

    public function delete(string $id): void
    {
        $this->client->request('DELETE', $this->path($id));
    }

    public function pause(string $id): Ingestion
    {
        return Ingestion::fromArray($this->client->request('POST', $this->path($id, '/pause')));
    }

    public function resume(string $id): Ingestion
    {
        return Ingestion::fromArray($this->client->request('POST', $this->path($id, '/resume')));
    }

    /**
     * @return IngestionVersion[]
     */
    public function versions(string $id): array
    {
        $response = $this->client->request('GET', $this->path($id, '/versions'));

        /** @var array<int, array<string, mixed>> $rows */
        $rows = $response['versions'] ?? [];

        return array_map(fn(array $row) => IngestionVersion::fromArray($row), $rows);
    }

    /**
     * Save a new version. It is not run until it is live: pass `$live = true`
     * (needs the promote permission) or call {@see makeVersionLive()}.
     */
    public function saveVersion(string $id, IngestionDefinition $definition, ?bool $live = null): IngestionVersion
    {
        $body = $definition->toArray();
        if ($live !== null) {
            $body['live'] = $live;
        }

        return IngestionVersion::fromArray(
            $this->client->request('POST', $this->path($id, '/versions'), ['json' => $body])
        );
    }

    public function makeVersionLive(string $id, int $version): Ingestion
    {
        return Ingestion::fromArray(
            $this->client->request('POST', $this->path($id, '/versions/' . $version . '/live'))
        );
    }

    /**
     * The definition file (YAML) that `fcdev ingest apply` reads back.
     */
    public function definition(string $id): string
    {
        $response = $this->client->request('GET', $this->path($id, '/definition'));

        return (string) ($response[FlowCatalystClient::RAW_BODY] ?? '');
    }

    /**
     * Audit history of one ingestion: `{entries, hasMore}`, newest first.
     *
     * @return array<string, mixed>
     */
    public function history(string $id, ?int $limit = null, ?int $offset = null): array
    {
        $query = $this->query(['limit' => $limit, 'offset' => $offset]);

        return $this->client->request('GET', $this->path($id, '/history') . $query);
    }

    /**
     * Failures a runtime reported: `{failures, hasMore, nextCursor?}`. Continue
     * with `$after = $page['nextCursor']`.
     *
     * @return array<string, mixed>
     */
    public function failures(string $id, ?string $after = null, ?int $limit = null): array
    {
        $query = $this->query(['after' => $after, 'limit' => $limit]);

        return $this->client->request('GET', $this->path($id, '/failures') . $query);
    }

    /**
     * Source kinds the platform knows plus the ones live runtimes report
     * (`custom` kinds have `builtIn` false).
     *
     * @return array<int, array<string, mixed>>
     */
    public function sourceKinds(): array
    {
        $response = $this->client->request('GET', '/api/ingestion-source-kinds');

        return $response['kinds'] ?? [];
    }

    /**
     * Queue a "test connection" (a real outbound call from a runtime; needs
     * the test permission). Test a saved ingestion by id, or a definition
     * without saving it (then `$application` and `$client` are required).
     * Poll the result with {@see getTest()}.
     *
     * @return array<string, mixed> The queued test (`status` PENDING)
     */
    public function testConnection(
        ?string $ingestionId = null,
        ?IngestionDefinition $definition = null,
        ?string $application = null,
        ?string $client = null,
    ): array {
        $body = array_filter([
            'ingestionId' => $ingestionId,
            'application' => $application,
            'client' => $client,
            'definition' => $definition?->toArray(),
        ], static fn($v) => $v !== null);

        return $this->client->request('POST', '/api/ingestion-tests', ['json' => (object) $body]);
    }

    /**
     * A test's state and, once `status` is DONE, its `outcome`.
     *
     * @return array<string, mixed>
     */
    public function getTest(string $id): array
    {
        return $this->client->request('GET', '/api/ingestion-tests/' . rawurlencode($id));
    }

    private function path(string $id, string $suffix = ''): string
    {
        return '/api/ingestions/' . rawurlencode($id) . $suffix;
    }

    /**
     * @param array<string, scalar|null> $params
     */
    private function query(array $params): string
    {
        $params = array_filter($params, static fn($v) => $v !== null);

        return $params !== [] ? '?' . http_build_query($params, '', '&', PHP_QUERY_RFC3986) : '';
    }
}
