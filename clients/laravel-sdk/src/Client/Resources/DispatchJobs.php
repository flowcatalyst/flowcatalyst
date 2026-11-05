<?php

declare(strict_types=1);

namespace FlowCatalyst\Client\Resources;

use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\DTOs\Responses\DispatchJobResult;
use FlowCatalyst\Exceptions\FlowCatalystException;
use FlowCatalyst\Outbox\DTOs\CreateDispatchJobDto;

/**
 * Creates dispatch jobs by calling the platform directly
 * (`POST /api/dispatch-jobs/batch`), for an application that does not write
 * them through its outbox. Each job is sent exactly as the outbox would send
 * it ({@see CreateDispatchJobDto::toBatchItem()}).
 *
 * The call needs `platform:messaging:batch:dispatch-jobs-write`. The batch is
 * all-or-nothing: an item the platform refuses (unknown connection, a client
 * the caller cannot reach, a bad target URL, ...) rejects the whole request,
 * which surfaces as a {@see FlowCatalystException}, and no job is created.
 *
 * Transient failures (429, 502-504, network errors) are retried by the
 * client. Set an idempotency key on each job so that a retried request whose
 * first attempt did land answers the same job instead of creating another.
 */
class DispatchJobs
{
    /** The most items the platform takes in one batch. */
    public const MAX_BATCH = 1000;

    public function __construct(
        private readonly FlowCatalystClient $client
    ) {}

    /**
     * Create jobs in one request. Returns one result per job, in order, as
     * the platform answered: the job's id (or, for an idempotency key already
     * held, the holding job's id) and its status.
     *
     * @param list<CreateDispatchJobDto> $jobs
     * @return list<DispatchJobResult>
     * @throws FlowCatalystException
     */
    public function batch(array $jobs): array
    {
        if ($jobs === []) {
            return [];
        }
        if (count($jobs) > self::MAX_BATCH) {
            throw new \InvalidArgumentException(
                'At most ' . self::MAX_BATCH . ' dispatch jobs per batch; got ' . count($jobs)
            );
        }

        $items = [];
        foreach (array_values($jobs) as $i => $job) {
            if (!$job instanceof CreateDispatchJobDto) {
                throw new \InvalidArgumentException("jobs[{$i}] is not a CreateDispatchJobDto");
            }
            $items[] = $job->toBatchItem();
        }

        $response = $this->client->request('POST', '/api/dispatch-jobs/batch', [
            'json' => ['items' => $items],
        ]);

        /** @var list<array<string, mixed>> $rows */
        $rows = $response['results'] ?? [];
        if (count($rows) !== count($items)) {
            throw FlowCatalystException::withContext(
                sprintf('Dispatch job batch answered %d results for %d items', count($rows), count($items)),
                $response,
            );
        }

        return array_map(static fn(array $row) => DispatchJobResult::fromArray($row), array_values($rows));
    }

    /**
     * Create one job and return its id (for a repeated idempotency key, the
     * id of the job that already holds it).
     *
     * @throws FlowCatalystException when the request fails or the platform
     *         reports the item as anything but SUCCESS
     */
    public function create(CreateDispatchJobDto $job): string
    {
        $result = $this->batch([$job])[0];
        if (!$result->isSuccess()) {
            throw FlowCatalystException::withContext(
                "Dispatch job {$job->code} was not created: {$result->status}"
                    . ($result->error !== null ? " ({$result->error})" : ''),
                ['id' => $result->id, 'status' => $result->status, 'error' => $result->error],
            );
        }

        return $result->id;
    }
}
