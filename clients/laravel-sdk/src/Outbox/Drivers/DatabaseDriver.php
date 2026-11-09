<?php

declare(strict_types=1);

namespace FlowCatalyst\Outbox\Drivers;

use FlowCatalyst\Exceptions\OutboxException;
use FlowCatalyst\Outbox\Contracts\OutboxDriver;
use FlowCatalyst\Outbox\DTOs\CreatePushDto;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Log;

/**
 * Database driver for MySQL 8.0+ and PostgreSQL 12+.
 *
 * Transactional safety
 * --------------------
 * Outbox writes are only atomic with your business writes if both happen
 * inside the same database transaction on the same connection. This driver
 * uses `DB::connection($name)`, which inherits whatever transaction stack
 * the connection currently holds — so wrapping your call in
 * `DB::transaction(fn () => ...)` makes the outbox row commit (or roll back)
 * with your business changes.
 *
 * The `$strictTransactions` flag controls what happens when no transaction
 * is active:
 *
 *  - `true`  — throw `OutboxException::noActiveTransaction()`. Use this in
 *              new code that should always be transactionally framed.
 *  - `false` — log a warning via `Log::warning(...)` and let the write
 *              proceed unfenced. This is the default for backwards
 *              compatibility; flip it on per environment when callers are
 *              ready.
 */
class DatabaseDriver implements OutboxDriver
{
    private const TABLE_PATTERN = '/^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$/D';

    public function __construct(
        private readonly ?string $connection,
        private readonly string $table = 'outbox_messages',
        private readonly bool $strictTransactions = false,
    ) {}

    /**
     * {@inheritdoc}
     */
    public function insert(array $message): void
    {
        $this->assertTransactionalContext();

        try {
            if ($this->notifiesOnInsert($message)) {
                $this->insertPush($message);
                return;
            }

            $this->getConnection()->table($this->table)->insert(
                $this->prepareMessage($message)
            );
        } catch (\Exception $e) {
            throw OutboxException::insertFailed($e->getMessage());
        }
    }

    /**
     * {@inheritdoc}
     */
    public function insertBatch(array $messages): void
    {
        if (empty($messages)) {
            return;
        }

        $this->assertTransactionalContext();

        try {
            $plain = [];
            foreach ($messages as $message) {
                if ($this->notifiesOnInsert($message)) {
                    $this->insertPush($message);
                } else {
                    $plain[] = $this->prepareMessage($message);
                }
            }

            if ($plain !== []) {
                $this->getConnection()->table($this->table)->insert($plain);
            }
        } catch (\Exception $e) {
            throw OutboxException::insertFailed($e->getMessage());
        }
    }

    /**
     * Whether this row is written with a notification: a PUSH row on Postgres.
     * Elsewhere (MySQL) the processor's poll delivers it.
     */
    private function notifiesOnInsert(array $message): bool
    {
        $connection = $this->getConnection();

        return $message['type'] === 'PUSH'
            && $connection instanceof \Illuminate\Database\Connection
            && $connection->getDriverName() === 'pgsql';
    }

    /**
     * The PUSH insert for Postgres: the same columns as the plain insert, written
     * in one statement with `pg_notify`, so the platform's outbox processor
     * (which LISTENs on the channel) delivers the push the moment the writer's
     * transaction commits and not before; a rollback notifies nobody. The poll
     * is the fallback for a processor that is not listening. Mirrors the Go
     * SDK's `outbox.PushInsertStatement`.
     *
     * @throws OutboxException when the table name is not a plain identifier
     */
    public function pushInsertStatement(string $tablePrefix = ''): string
    {
        if (preg_match(self::TABLE_PATTERN, $this->table) !== 1) {
            throw OutboxException::insertFailed("invalid table name '{$this->table}'");
        }
        $parts = explode('.', $this->table);
        $parts[count($parts) - 1] = $tablePrefix . $parts[count($parts) - 1];
        $table = implode('.', array_map(static fn(string $p) => '"' . $p . '"', $parts));

        return "WITH woken AS (SELECT pg_notify('" . CreatePushDto::NOTIFY_CHANNEL . "', ''))\n"
            . "INSERT INTO {$table} (\n"
            . "  id, type, message_group, payload, status, created_at, updated_at,\n"
            . "  client_id, payload_size, headers\n"
            . ")\n"
            . "SELECT ?::text, ?::text, ?::text, ?::text, ?::smallint, ?::timestamptz, ?::timestamptz,\n"
            . "  ?::text, ?::integer, ?::jsonb\n"
            . "FROM woken";
    }

    private function insertPush(array $message): void
    {
        $row = $this->prepareMessage($message);
        $connection = $this->getConnection();
        $prefix = $connection instanceof \Illuminate\Database\Connection ? $connection->getTablePrefix() : '';
        $connection->insert($this->pushInsertStatement($prefix), [
            $row['id'],
            $row['type'],
            $row['message_group'],
            $row['payload'],
            $row['status'],
            $row['created_at'],
            $row['updated_at'],
            $row['client_id'],
            $row['payload_size'],
            $row['headers'],
        ]);
    }

    /**
     * Refuse (strict mode) or warn (default) when the outbox is written
     * outside of an active database transaction on this connection. See
     * the class-level doc for the rationale.
     */
    private function assertTransactionalContext(): void
    {
        if ($this->getConnection()->transactionLevel() > 0) {
            return;
        }

        if ($this->strictTransactions) {
            throw OutboxException::noActiveTransaction($this->connection);
        }

        Log::warning(
            'FlowCatalyst outbox write outside of DB::transaction(...). '
            . 'Business writes and outbox writes will NOT commit atomically — '
            . 'enable flowcatalyst.outbox.strict_transactions to make this an error.',
            ['connection' => $this->connection ?? '(default)'],
        );
    }

    /**
     * Prepare a message for database insertion.
     * Column layout matches the outbox poller's schema.
     */
    private function prepareMessage(array $message): array
    {
        return [
            // Processor-required columns
            'id' => $message['id'],
            'type' => $message['type'],
            'message_group' => $message['message_group'] ?? null,
            'payload' => $message['payload'],
            'status' => $message['status'],
            'created_at' => $message['created_at'],
            'updated_at' => $message['updated_at'],
            // SDK-specific columns
            'client_id' => $message['client_id'],
            'payload_size' => $message['payload_size'],
            'headers' => isset($message['headers']) ? json_encode($message['headers']) : null,
        ];
    }

    /**
     * Get the database connection.
     */
    private function getConnection(): \Illuminate\Database\ConnectionInterface
    {
        return DB::connection($this->connection);
    }
}
