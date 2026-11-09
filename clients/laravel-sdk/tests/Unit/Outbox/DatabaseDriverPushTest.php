<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Unit\Outbox;

use FlowCatalyst\Outbox\DTOs\CreateEventDto;
use FlowCatalyst\Outbox\DTOs\CreatePushDto;
use FlowCatalyst\Outbox\Drivers\DatabaseDriver;
use FlowCatalyst\Outbox\OutboxManager;
use Illuminate\Database\Connection;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Schema;
use Orchestra\Testbench\TestCase;

/**
 * PUSH rows through the real DatabaseDriver: on Postgres the insert runs
 * `pg_notify('fc_outbox_push')` in the same statement (the platform's outbox
 * processor wakes at commit); elsewhere it is the plain row and the processor's
 * poll delivers. The Postgres statement is checked as text (no Postgres here);
 * the plain path runs against SQLite.
 */
final class DatabaseDriverPushTest extends TestCase
{
    protected function defineEnvironment($app): void
    {
        $app['config']->set('database.default', 'testing');
        $app['config']->set('database.connections.testing', [
            'driver' => 'sqlite',
            'database' => ':memory:',
            'prefix' => '',
        ]);
    }

    private function createTable(): void
    {
        Schema::create('outbox_messages', function ($table) {
            $table->string('id')->primary();
            $table->string('type');
            $table->string('message_group')->nullable();
            $table->text('payload');
            $table->smallInteger('status');
            $table->string('created_at');
            $table->string('updated_at');
            $table->string('client_id');
            $table->integer('payload_size');
            $table->text('headers')->nullable();
        });
    }

    public function test_a_push_is_a_plain_row_off_postgres_alongside_other_rows(): void
    {
        $this->createTable();
        $outbox = new OutboxManager(new DatabaseDriver(null), 'cli_1', 'acme');

        DB::transaction(function () use ($outbox): void {
            $outbox->createPushes([
                CreatePushDto::create('shop:orders:ord_1', 'order.updated', ['status' => 'placed'])->withId('P1'),
                CreatePushDto::create('shop:board', 'refresh', [])->withId('P2')->withUngrouped(),
            ]);
            $outbox->createEvent(CreateEventDto::create('shop:orders:order:placed', ['a' => 1]));
        });

        $rows = DB::table('outbox_messages')->orderBy('created_at')->get()->keyBy('id');
        $this->assertCount(3, $rows);
        $this->assertSame('PUSH', $rows['P1']->type);
        $this->assertSame('shop:orders:ord_1', $rows['P1']->message_group);
        $this->assertSame(
            '{"topic":"shop:orders:ord_1","type":"order.updated","data":{"status":"placed"},"clientCode":"acme","pushId":"P1"}',
            $rows['P1']->payload,
        );
        $this->assertNull($rows['P2']->message_group);
        $this->assertSame(0, (int) $rows['P1']->status);
    }

    public function test_a_rolled_back_transaction_leaves_no_push(): void
    {
        $this->createTable();
        $outbox = new OutboxManager(new DatabaseDriver(null), 'cli_1');
        try {
            DB::transaction(function () use ($outbox): void {
                $outbox->createPush(CreatePushDto::create('shop:board', 'refresh', []));
                throw new \RuntimeException('business failure');
            });
        } catch (\RuntimeException) {
        }
        $this->assertSame(0, DB::table('outbox_messages')->count());
    }

    public function test_the_postgres_statement_is_the_go_sdks_push_insert_statement(): void
    {
        $sql = (new DatabaseDriver(null))->pushInsertStatement();
        // Go's PushInsertStatement uses $1..$10; PDO takes ? placeholders. Compare with them numbered.
        $n = 0;
        $numbered = preg_replace_callback('/\?/', static function () use (&$n): string {
            return '$' . ++$n;
        }, $sql);
        $this->assertSame(10, $n);
        $norm = static fn(string $s): string => trim((string) preg_replace('/\s+/', ' ', str_replace('"', '', $s)));
        $this->assertSame(
            $norm("WITH woken AS (SELECT pg_notify('fc_outbox_push', ''))
INSERT INTO outbox_messages (
  id, type, message_group, payload, status, created_at, updated_at,
  client_id, payload_size, headers
)
SELECT \$1::text, \$2::text, \$3::text, \$4::text, \$5::smallint, \$6::timestamptz, \$7::timestamptz,
  \$8::text, \$9::integer, \$10::jsonb
FROM woken"),
            $norm((string) $numbered),
        );
    }

    public function test_the_postgres_statement_quotes_and_prefixes_the_table_and_refuses_odd_names(): void
    {
        $this->assertStringContainsString('INSERT INTO "app"."pre_outbox" (', (new DatabaseDriver(null, 'app.outbox'))->pushInsertStatement('pre_'));
        $this->expectException(\FlowCatalyst\Exceptions\OutboxException::class);
        (new DatabaseDriver(null, 'outbox; DROP TABLE x'))->pushInsertStatement();
    }

    public function test_on_postgres_a_push_runs_the_notify_statement_and_other_rows_the_plain_insert(): void
    {
        $connection = \Mockery::mock(Connection::class);
        $connection->shouldReceive('transactionLevel')->andReturn(1);
        $connection->shouldReceive('getDriverName')->andReturn('pgsql');
        $connection->shouldReceive('getTablePrefix')->andReturn('');
        $statements = [];
        $connection->shouldReceive('insert')->andReturnUsing(function (string $sql, array $bindings) use (&$statements) {
            $statements[] = [$sql, $bindings];
            return true;
        });
        $plainTable = \Mockery::mock();
        $plainRows = [];
        $plainTable->shouldReceive('insert')->andReturnUsing(function (array $rows) use (&$plainRows) {
            $plainRows[] = $rows;
            return true;
        });
        $connection->shouldReceive('table')->with('outbox_messages')->andReturn($plainTable);
        DB::shouldReceive('connection')->andReturn($connection);

        $outbox = new OutboxManager(new DatabaseDriver(null), 'cli_1');
        $outbox->createPush(CreatePushDto::create('shop:board', 'refresh', [])->withId('P1'));
        $outbox->createEvent(CreateEventDto::create('shop:orders:order:placed', ['a' => 1]));

        $this->assertCount(1, $statements);
        $this->assertStringStartsWith("WITH woken AS (SELECT pg_notify('fc_outbox_push', ''))", $statements[0][0]);
        $this->assertSame('P1', $statements[0][1][0]);
        $this->assertSame('PUSH', $statements[0][1][1]);
        $this->assertCount(10, $statements[0][1]);
        $this->assertCount(1, $plainRows, 'the event is the plain insert');
        $this->assertSame('EVENT', $plainRows[0]['type']);
    }
}
