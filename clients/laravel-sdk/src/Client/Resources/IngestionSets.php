<?php

declare(strict_types=1);

namespace FlowCatalyst\Client\Resources;

use FlowCatalyst\Client\FlowCatalystClient;

/**
 * Ingestion sets (`/api/ingestion-sets...`): named sets of definition files.
 * A set owns its ingestions until it is released; an ingestion owned by
 * another set is a 409 `INGESTION_SET_OWNED` and nothing is written. An
 * ingestion a set owns but no longer lists is deleted by the next apply.
 * Needs `platform:ingestion:set:apply`. Path segments are URL-encoded here.
 *
 * Items are decoded arrays: each has `application`, `client` (id or
 * identifier), `code`, `name`, `source`, `split`, `target` and optionally
 * `active` and `keepFailedPayloads`.
 */
class IngestionSets
{
    public function __construct(
        private readonly FlowCatalystClient $client
    ) {}

    /**
     * @return array<int, array<string, mixed>>
     */
    public function list(): array
    {
        return $this->client->request('GET', '/api/ingestion-sets')['sets'] ?? [];
    }

    /**
     * Apply a set in one transaction. `$dryRun` returns the plan (CREATE,
     * UNCHANGED, CHANGE, ADOPT, DELETE per item) and writes nothing.
     *
     * @param array<int, array<string, mixed>> $items
     * @param array<int, array{path: string, content: string}>|null $files The files the items were read from, kept with the apply
     * @return array<string, mixed> The plan: `{name, dryRun, changes, applyId?}`
     */
    public function apply(string $name, array $items, ?array $files = null, bool $dryRun = false): array
    {
        $body = ['items' => $items];
        if ($files !== null) {
            $body['files'] = $files;
        }

        return $this->client->request(
            'PUT',
            $this->path($name) . $this->dryRunQuery($dryRun),
            ['json' => $body],
        );
    }

    /**
     * @return array<int, array<string, mixed>>
     */
    public function applies(string $name): array
    {
        return $this->client->request('GET', $this->path($name, '/applies'))['applies'] ?? [];
    }

    /**
     * @return array<string, mixed>
     */
    public function getApply(string $name, string $applyId): array
    {
        return $this->client->request('GET', $this->path($name, '/applies/' . rawurlencode($applyId)));
    }

    /**
     * Apply an earlier apply's items again.
     *
     * @return array<string, mixed> The plan, as {@see apply()}
     */
    public function reapply(string $name, string $applyId, bool $dryRun = false): array
    {
        return $this->client->request(
            'POST',
            $this->path($name, '/applies/' . rawurlencode($applyId) . '/reapply') . $this->dryRunQuery($dryRun),
        );
    }

    /**
     * Release the set's ingestions; they stay, no longer owned by the set.
     *
     * @return array<string, mixed> `{name, released}`
     */
    public function release(string $name): array
    {
        return $this->client->request('POST', $this->path($name, '/release'));
    }

    private function path(string $name, string $suffix = ''): string
    {
        return '/api/ingestion-sets/' . rawurlencode($name) . $suffix;
    }

    private function dryRunQuery(bool $dryRun): string
    {
        return $dryRun ? '?dryRun=true' : '';
    }
}
