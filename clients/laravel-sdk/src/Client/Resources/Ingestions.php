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
     * first version without making it live, and so does a caller without the
     * promote permission whatever `$live` says: the answer's `save->outcome`
     * is `DRAFT` and `save->awaitingPromote` is true.
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
     * Save the definition as the next version: made live with the promote
     * permission (`SAVED`), a draft awaiting promote without it (`DRAFT`).
     * `$expectedVersion` is the live version the caller read
     * (`Ingestion::$version`, 0 when none is live); a different current
     * version is a 409. A definition an existing version already says is not
     * saved again (`UNCHANGED`, `MADE_LIVE`, `DRAFT_EXISTS`); `save` says which.
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
     * (needs the promote permission; without it the save is a draft) or call
     * {@see makeVersionLive()}. A definition an existing version already says
     * is not saved again; `save` on the answer says what happened.
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
     * Failures, newest first: `{failures, hasMore, nextCursor?}`. Continue
     * with `$after = $page['nextCursor']`. `$category` is `payload` or
     * `rejected` (the notices); `$undismissed` keeps only the failures nobody
     * has dismissed. `category: 'rejected', undismissed: true` lists the
     * ingestion's open notices (what `Ingestion::$notices` counts).
     *
     * @return array<string, mixed>
     */
    public function failures(
        string $id,
        ?string $after = null,
        ?int $limit = null,
        ?string $category = null,
        bool $undismissed = false,
    ): array {
        $query = $this->query([
            'after' => $after,
            'limit' => $limit,
            'category' => $category,
            'undismissed' => $undismissed ? 'true' : null,
        ]);

        return $this->client->request('GET', $this->path($id, '/failures') . $query);
    }

    /**
     * Dismiss a failure: a notice stops showing on the ingestion; the failure
     * stays listed. Returns the failure (`dismissedAt`, `dismissedBy` set).
     *
     * @return array<string, mixed>
     */
    public function dismissFailure(string $id, string $failureId): array
    {
        return $this->client->request(
            'POST',
            $this->path($id, '/failures/' . rawurlencode($failureId) . '/dismiss'),
        );
    }

    /**
     * The secret names the live definition binds and, per runtime, when it
     * last fetched each one: `{secrets: [{name, bound, fetches: [{runtimeId,
     * version, principalId?, lastFetchedAt, fetchCount}]}]}`. Names only: no
     * secret value is ever returned.
     *
     * @return array<int, array<string, mixed>>
     */
    public function secrets(string $id): array
    {
        $response = $this->client->request('GET', $this->path($id, '/secrets'));

        return $response['secrets'] ?? [];
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
