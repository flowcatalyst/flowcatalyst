<?php

declare(strict_types=1);

namespace FlowCatalyst\Outbox;

use FlowCatalyst\Enums\MessageType;
use FlowCatalyst\Exceptions\OutboxException;
use FlowCatalyst\Outbox\Contracts\OutboxDriver;
use FlowCatalyst\Outbox\DTOs\CreateDispatchJobDto;
use FlowCatalyst\Outbox\DTOs\CreateEventDto;
use FlowCatalyst\Support\TsidGenerator;

class OutboxManager
{
    public function __construct(
        private readonly OutboxDriver $driver,
        private readonly int $tenantId,
        private readonly string $defaultPartition = 'default'
    ) {}

    /**
     * Create an event in the outbox.
     *
     * @return string The generated message ID (TSID)
     * @throws OutboxException
     */
    public function createEvent(CreateEventDto $event): string
    {
        $this->ensureTenantId();

        $id = TsidGenerator::generate();
        $payload = json_encode($event->toPayload());

        $message = [
            'id' => $id,
            'tenant_id' => $this->tenantId,
            'partition_id' => $event->partitionId ?: $this->defaultPartition,
            'type' => MessageType::EVENT->value,
            'payload' => $payload,
            'payload_size' => strlen($payload),
            'status' => 'PENDING',
            'created_at' => date('Y-m-d H:i:s'),
            'headers' => !empty($event->headers) ? $event->headers : null,
        ];

        $this->driver->insert($message);

        return $id;
    }

    /**
     * Create a dispatch job in the outbox.
     *
     * @return string The generated message ID (TSID)
     * @throws OutboxException
     */
    public function createDispatchJob(CreateDispatchJobDto $job): string
    {
        $this->ensureTenantId();

        $id = TsidGenerator::generate();
        $payload = json_encode($job->toPayload());

        $message = [
            'id' => $id,
            'tenant_id' => $this->tenantId,
            'partition_id' => $job->partitionId ?: $this->defaultPartition,
            'type' => MessageType::DISPATCH_JOB->value,
            'payload' => $payload,
            'payload_size' => strlen($payload),
            'status' => 'PENDING',
            'created_at' => date('Y-m-d H:i:s'),
            'headers' => null,
        ];

        $this->driver->insert($message);

        return $id;
    }

    /**
     * Create multiple events in the outbox (batch).
     *
     * @param CreateEventDto[] $events
     * @return string[] The generated message IDs (TSIDs)
     * @throws OutboxException
     */
    public function createEvents(array $events): array
    {
        if (empty($events)) {
            return [];
        }

        $this->ensureTenantId();

        $messages = [];
        $ids = [];

        foreach ($events as $event) {
            $id = TsidGenerator::generate();
            $ids[] = $id;

            $payload = json_encode($event->toPayload());

            $messages[] = [
                'id' => $id,
                'tenant_id' => $this->tenantId,
                'partition_id' => $event->partitionId ?: $this->defaultPartition,
                'type' => MessageType::EVENT->value,
                'payload' => $payload,
                'payload_size' => strlen($payload),
                'status' => 'PENDING',
                'created_at' => date('Y-m-d H:i:s'),
                'headers' => !empty($event->headers) ? $event->headers : null,
            ];
        }

        $this->driver->insertBatch($messages);

        return $ids;
    }

    /**
     * Create multiple dispatch jobs in the outbox (batch).
     *
     * @param CreateDispatchJobDto[] $jobs
     * @return string[] The generated message IDs (TSIDs)
     * @throws OutboxException
     */
    public function createDispatchJobs(array $jobs): array
    {
        if (empty($jobs)) {
            return [];
        }

        $this->ensureTenantId();

        $messages = [];
        $ids = [];

        foreach ($jobs as $job) {
            $id = TsidGenerator::generate();
            $ids[] = $id;

            $payload = json_encode($job->toPayload());

            $messages[] = [
                'id' => $id,
                'tenant_id' => $this->tenantId,
                'partition_id' => $job->partitionId ?: $this->defaultPartition,
                'type' => MessageType::DISPATCH_JOB->value,
                'payload' => $payload,
                'payload_size' => strlen($payload),
                'status' => 'PENDING',
                'created_at' => date('Y-m-d H:i:s'),
                'headers' => null,
            ];
        }

        $this->driver->insertBatch($messages);

        return $ids;
    }

    /**
     * Get the underlying driver.
     */
    public function driver(): OutboxDriver
    {
        return $this->driver;
    }

    /**
     * Ensure tenant ID is configured.
     *
     * @throws OutboxException
     */
    private function ensureTenantId(): void
    {
        if ($this->tenantId <= 0) {
            throw OutboxException::missingTenantId();
        }
    }
}
