<?php

declare(strict_types=1);

namespace FlowCatalyst\Outbox\Contracts;

interface OutboxDriver
{
    /**
     * Insert a single message into the outbox.
     *
     * @param array{
     *     id: string,
     *     tenant_id: int,
     *     partition_id: string,
     *     type: string,
     *     payload: string,
     *     payload_size: int,
     *     status: string,
     *     created_at: string,
     *     headers: array|null
     * } $message
     */
    public function insert(array $message): void;

    /**
     * Insert multiple messages into the outbox.
     *
     * @param array<array{
     *     id: string,
     *     tenant_id: int,
     *     partition_id: string,
     *     type: string,
     *     payload: string,
     *     payload_size: int,
     *     status: string,
     *     created_at: string,
     *     headers: array|null
     * }> $messages
     */
    public function insertBatch(array $messages): void;
}
