<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Unit\UseCase;

use FlowCatalyst\Outbox\DTOs\CreatePushDto;
use FlowCatalyst\Outbox\Drivers\DatabaseDriver;
use FlowCatalyst\Outbox\OutboxManager;
use FlowCatalyst\UseCase\Aggregate;
use FlowCatalyst\UseCase\BaseDomainEvent;
use FlowCatalyst\UseCase\Command;
use FlowCatalyst\UseCase\DomainEvent;
use FlowCatalyst\UseCase\DomainEventHelpers;
use FlowCatalyst\UseCase\ExecutionContext;
use FlowCatalyst\UseCase\Operation;
use FlowCatalyst\UseCase\OutboxUnitOfWork;
use FlowCatalyst\UseCase\Plan;
use FlowCatalyst\UseCase\Repo;
use FlowCatalyst\UseCase\Runner;
use FlowCatalyst\UseCase\UseCaseError;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Schema;
use Orchestra\Testbench\TestCase;

/**
 * Plan::andPush against the real OutboxUnitOfWork on SQLite: the aggregate row,
 * the EVENT row and the PUSH rows commit together, and an invalid push rolls
 * the lot back.
 */
final class PlanPushTest extends TestCase
{
    protected function defineEnvironment($app): void
    {
        $app['config']->set('database.default', 'testing');
        $app['config']->set('database.connections.testing', ['driver' => 'sqlite', 'database' => ':memory:', 'prefix' => '']);
    }

    protected function setUp(): void
    {
        parent::setUp();
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
        Schema::create('push_orders', function ($table) {
            $table->string('id')->primary();
        });
    }

    private function uow(): OutboxUnitOfWork
    {
        return new OutboxUnitOfWork(
            outboxManager: new OutboxManager(new DatabaseDriver(null), 'cli_1'),
            clientCode: 'acme',
        );
    }

    private function operation(callable $pushes): Operation
    {
        return new class ($pushes) implements Operation {
            public function __construct(private $pushes) {}

            public function validate(Command $command): ?UseCaseError
            {
                return null;
            }

            public function authorize(Command $command, ExecutionContext $context): ?UseCaseError
            {
                return null;
            }

            public function execute(Command $command, ExecutionContext $context): Plan|UseCaseError
            {
                $order = new PushOrder('ord_1');
                $event = new PushOrderPlaced($context, $order->id());
                return Plan::save($order, new PushOrderRepo(), $event)->andPush(...($this->pushes)($event));
            }
        };
    }

    private function runPlan(callable $pushes): \FlowCatalyst\UseCase\Result
    {
        return Runner::run($this->uow(), $this->operation($pushes), new class implements Command {}, ExecutionContext::create('prn_test'));
    }

    public function test_pushes_are_written_after_the_event_in_the_plans_transaction(): void
    {
        $result = $this->runPlan(fn(DomainEvent $e) => [
            CreatePushDto::create('shop:orders:ord_1', 'order.updated', ['status' => 'placed']),
            fn(DomainEvent $ev) => CreatePushDto::create('shop:board', 'refresh', ['eventId' => $ev->eventId()])->withUngrouped(),
        ]);

        $this->assertTrue($result->isSuccess());
        $this->assertSame(1, DB::table('push_orders')->count());
        $rows = DB::table('outbox_messages')->get();
        $types = $rows->pluck('type')->sort()->values()->all();
        $this->assertSame(['EVENT', 'PUSH', 'PUSH'], $types);

        $pushes = $rows->where('type', 'PUSH')->values();
        $byTopic = [];
        foreach ($pushes as $row) {
            $byTopic[json_decode($row->payload)->topic] = $row;
        }
        $first = $byTopic['shop:orders:ord_1'];
        $this->assertSame('shop:orders:ord_1', $first->message_group);
        $this->assertSame('acme', json_decode($first->payload)->clientCode, "the unit of work's client code");
        $board = $byTopic['shop:board'];
        $this->assertNull($board->message_group);
        $this->assertSame($result->value()->eventId(), json_decode($board->payload)->data->eventId);
    }

    public function test_an_invalid_push_fails_the_plan_and_rolls_everything_back(): void
    {
        $result = $this->runPlan(fn(DomainEvent $e) => [CreatePushDto::create('shop:orders:ord_1', 'fw.reserved', [])]);

        $this->assertTrue($result->isFailure());
        $this->assertSame('TYPE_RESERVED', $result->error()->code);
        $this->assertSame(0, DB::table('outbox_messages')->count(), 'no event, no push');
        $this->assertSame(0, DB::table('push_orders')->count(), 'aggregate rolled back');
    }

    public function test_a_plan_without_pushes_is_unchanged(): void
    {
        $result = $this->runPlan(fn(DomainEvent $e) => []);
        $this->assertTrue($result->isSuccess());
        $this->assertSame(['EVENT'], DB::table('outbox_messages')->pluck('type')->all());
    }

    public function test_a_unit_of_work_that_cannot_write_pushes_fails_the_plan(): void
    {
        $uow = new class implements \FlowCatalyst\UseCase\UnitOfWork {
            public function transaction(callable $callback): mixed
            {
                return $callback();
            }

            public function commit(DomainEvent $event, mixed $command, ?callable $persist = null): \FlowCatalyst\UseCase\Result
            {
                return \FlowCatalyst\UseCase\Result::success(\FlowCatalyst\UseCase\ResultSuccessToken::internal(), $event);
            }

            public function commitAggregate(Aggregate $aggregate, DomainEvent $event, mixed $command, ?callable $persist = null): \FlowCatalyst\UseCase\Result
            {
                return $this->commit($event, $command);
            }

            public function commitDelete(Aggregate $aggregate, DomainEvent $event, mixed $command, ?callable $persist = null): \FlowCatalyst\UseCase\Result
            {
                return $this->commit($event, $command);
            }

            public function emitEvent(DomainEvent $event, mixed $command): \FlowCatalyst\UseCase\Result
            {
                return $this->commit($event, $command);
            }
        };
        $result = Runner::run(
            $uow,
            $this->operation(fn(DomainEvent $e) => [CreatePushDto::create('shop:board', 'refresh', [])]),
            new class implements Command {},
            ExecutionContext::create('prn_test'),
        );
        $this->assertTrue($result->isFailure());
        $this->assertSame('PUSH_UNSUPPORTED', $result->error()->code);
    }
}

final class PushOrder implements Aggregate
{
    public function __construct(private readonly string $id) {}

    public function id(): string
    {
        return $this->id;
    }
}

final class PushOrderRepo implements Repo
{
    public function persist(Aggregate $aggregate): void
    {
        DB::table('push_orders')->insert(['id' => $aggregate->id()]);
    }

    public function delete(Aggregate $aggregate): void
    {
        DB::table('push_orders')->where('id', $aggregate->id())->delete();
    }
}

/**
 * @extends BaseDomainEvent<array{orderId: string}>
 */
final class PushOrderPlaced extends BaseDomainEvent
{
    public function __construct(ExecutionContext $ctx, string $orderId)
    {
        parent::__construct(
            [
                'eventType' => 'shop:orders:order:placed',
                'specVersion' => '1.0',
                'source' => 'shop:orders',
                'subject' => DomainEventHelpers::subject('orders', 'order', $orderId),
                'messageGroup' => DomainEventHelpers::messageGroup('orders', 'order', $orderId),
            ],
            $ctx,
            ['orderId' => $orderId],
        );
    }
}
