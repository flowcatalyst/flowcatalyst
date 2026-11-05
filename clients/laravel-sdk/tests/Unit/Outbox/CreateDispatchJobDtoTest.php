<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Unit\Outbox;

use FlowCatalyst\Outbox\DTOs\CreateDispatchJobDto;
use PHPUnit\Framework\TestCase;

final class CreateDispatchJobDtoTest extends TestCase
{
    public function test_queue_is_omitted_when_unset_but_carried_when_set(): void
    {
        $base = CreateDispatchJobDto::create(
            source: 'svc',
            code: 'app:sub:agg:act',
            targetUrl: 'https://example.com/hook',
            payload: '{"k":1}',
            dispatchPoolId: 'pool-1',
        );

        $this->assertArrayNotHasKey(
            'queue',
            $base->toPayload(),
            'an unset queue must be omitted, never defaulted',
        );

        $withQueue = $base->withQueue('HIGH_PRIORITY');
        $this->assertSame('HIGH_PRIORITY', $withQueue->toPayload()['queue']);
    }

    /**
     * Every wither hand-copies all fields into a new instance, so one that
     * forgets `queue` drops it the moment it is chained after withQueue() —
     * invisible to a test that only calls withQueue() last. Chains through a
     * later wither instead.
     */
    public function test_queue_survives_a_later_wither(): void
    {
        $job = CreateDispatchJobDto::create(
            source: 'svc',
            code: 'app:sub:agg:act',
            targetUrl: 'https://example.com/hook',
            payload: '{"k":1}',
            dispatchPoolId: 'pool-1',
        )->withQueue('HIGH_PRIORITY')->withMessageGroup('grp-1');

        $payload = $job->toPayload();
        $this->assertSame('HIGH_PRIORITY', $payload['queue'], 'a later wither must not drop queue');
        $this->assertSame('grp-1', $payload['messageGroup']);
    }

    private function job(): CreateDispatchJobDto
    {
        return CreateDispatchJobDto::create(
            source: 'svc',
            code: 'app:sub:agg:act',
            targetUrl: 'https://example.com/hook',
            payload: '{"k":1}',
            dispatchPoolId: 'pool-1',
        );
    }

    /**
     * The platform's batch takes metadata as a list of {key, value}; an
     * object fails the whole batch. An associative array is converted
     * (key-sorted, values as strings); a list of pairs is sent as given.
     */
    public function test_metadata_is_sent_as_a_key_value_list(): void
    {
        $item = $this->job()
            ->withMetadata(['tenant' => 'acme', 'attempt' => 2, 'orderId' => 'o-1', 'flag' => true, 'none' => null])
            ->toBatchItem();

        $this->assertSame([
            ['key' => 'attempt', 'value' => '2'],
            ['key' => 'flag', 'value' => 'true'],
            ['key' => 'none', 'value' => ''],
            ['key' => 'orderId', 'value' => 'o-1'],
            ['key' => 'tenant', 'value' => 'acme'],
        ], $item['metadata']);
        $this->assertTrue(array_is_list($item['metadata']));
        $this->assertSame(
            '[{"key":"attempt","value":"2"}',
            substr((string) json_encode($item['metadata']), 0, 30),
            'encodes as a JSON array, never an object',
        );

        $pairs = [['key' => 'b', 'value' => '1'], ['key' => 'a', 'value' => '2']];
        $this->assertSame($pairs, $this->job()->withMetadata($pairs)->toBatchItem()['metadata'], 'a list is kept as given');

        $this->assertArrayNotHasKey('metadata', $this->job()->toBatchItem(), 'no metadata, no key');
    }

    public function test_client_connection_and_idempotency_key_are_sent(): void
    {
        $item = $this->job()
            ->withClientCode('acme')
            ->withClientId('clt_0000000000001')
            ->withConnectionId('con_0000000000001')
            ->withIdempotencyKey('order-1')
            ->withMessageGroup('grp') // a later wither keeps them all
            ->toBatchItem();

        $this->assertSame('acme', $item['clientCode']);
        $this->assertSame('clt_0000000000001', $item['clientId']);
        $this->assertSame('con_0000000000001', $item['connectionId']);
        $this->assertSame('order-1', $item['idempotencyKey']);
        $this->assertSame('grp', $item['messageGroup']);

        $bare = $this->job()->toBatchItem();
        foreach (['clientCode', 'clientId', 'connectionId', 'idempotencyKey'] as $absent) {
            $this->assertArrayNotHasKey($absent, $bare);
        }
    }

    /**
     * headers, retryStrategy, scheduledFor and expiresAt are not honoured by
     * the batch route, so they are not sent (the withers stay for callers).
     */
    public function test_fields_the_platform_does_not_honour_are_not_sent(): void
    {
        $job = new CreateDispatchJobDto(
            source: 'svc',
            code: 'app:sub:agg:act',
            targetUrl: 'https://example.com/hook',
            payload: '{}',
            dispatchPoolId: 'pool-1',
            retryStrategy: 'fixed',
        );
        $item = $job->withHeaders(['X-A' => 'b'])
            ->scheduledFor(new \DateTimeImmutable('2030-01-01T00:00:00Z'))
            ->expiresAt(new \DateTimeImmutable('2030-01-02T00:00:00Z'))
            ->toBatchItem();

        foreach (['headers', 'retryStrategy', 'scheduledFor', 'expiresAt'] as $dropped) {
            $this->assertArrayNotHasKey($dropped, $item);
        }
    }

    /** The outbox row and the direct API call send the same item. */
    public function test_outbox_payload_is_the_batch_item(): void
    {
        $job = $this->job()->withMetadata(['a' => 'b'])->withConnectionId('con_1')->withClientCode('acme');
        $this->assertSame($job->toBatchItem(), $job->toPayload());
    }

    public function test_an_empty_dispatch_pool_id_is_omitted(): void
    {
        $job = CreateDispatchJobDto::create('svc', 'app:sub:agg:act', 'https://example.com/hook', '{}', '');
        $this->assertArrayNotHasKey('dispatchPoolId', $job->toBatchItem());
    }
}
