<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Unit\Outbox;

use FlowCatalyst\Exceptions\PushException;
use FlowCatalyst\Outbox\Contracts\OutboxDriver;
use FlowCatalyst\Outbox\DTOs\CreatePushDto;
use FlowCatalyst\Outbox\OutboxManager;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

/**
 * The shared vector `outbox-push.json` (a pinned copy of the framework's
 * conformance/outbox/outbox-push.json, which the Go SDK pins as well): each case
 * is the writer's context, the pushes and the rows read back; `invalidPushes`
 * are builder inputs and the refusal code.
 *
 * The rows' timestamps are not compared: OutboxManager stamps the wall clock
 * (`date()`), as for every other row type, and the vector fixes a clock.
 */
final class PushOutboxConformanceTest extends TestCase
{
    private static function vector(): object
    {
        return json_decode((string) file_get_contents(__DIR__ . '/../../Fixtures/push/outbox-push.json'), false, 512, JSON_THROW_ON_ERROR);
    }

    /** @return array<string, array{object}> */
    public static function cases(): array
    {
        $out = [];
        foreach (self::vector()->cases as $case) {
            $out[$case->name] = [$case];
        }
        return $out;
    }

    /** @return array<string, array{object}> */
    public static function invalidPushes(): array
    {
        $out = [];
        foreach (self::vector()->invalidPushes as $i => $bad) {
            $out["#{$i} {$bad->code} " . ($bad->topic) . ' ' . ($bad->type)] = [$bad];
        }
        return $out;
    }

    private static function build(object $v): CreatePushDto
    {
        $data = $v->data ?? [];
        if (isset($v->dataText)) {
            $data = $v->dataText;
        } elseif (isset($v->padBytes)) {
            $data = ['blob' => str_repeat('x', $v->padBytes)];
        }
        $push = CreatePushDto::create($v->topic, $v->type, $data);
        if (!empty($v->id)) {
            $push = $push->withId($v->id);
        }
        if (!empty($v->clientCode)) {
            $push = $push->withClientCode($v->clientCode);
        }
        if (!empty($v->ungrouped)) {
            $push = $push->withUngrouped();
        }
        if (!empty($v->messageGroup)) {
            $push = $push->withMessageGroup($v->messageGroup);
        }
        return $push;
    }

    #[DataProvider('cases')]
    public function test_the_rows_are_the_vectors(object $case): void
    {
        $driver = new RecordingDriver();
        // The writer's client: the row client_id and the default clientCode (`_platform` leaves it out).
        $outbox = new OutboxManager($driver, $case->context->client, $case->context->client);
        foreach ($case->pushes as $push) {
            $outbox->createPush(self::build($push));
        }

        $want = array_values(array_filter($case->rows, static fn($r) => $r->type === 'PUSH'));
        $this->assertCount(count($want), $driver->rows);
        foreach ($driver->rows as $i => $got) {
            $w = $want[$i];
            $this->assertSame($w->id, $got['id']);
            $this->assertSame('PUSH', $got['type']);
            $this->assertSame($w->message_group, $got['message_group']);
            $this->assertSame($w->status, $got['status']);
            $this->assertSame($w->client_id, $got['client_id']);
            $this->assertNull($got['headers']);
            $this->assertSame($w->payloadText, $got['payload'], 'payload bytes, members in order');
            $this->assertSame($w->payload_size, $got['payload_size']);
            $this->assertEquals($w->payload, json_decode($got['payload']));
        }
    }

    #[DataProvider('invalidPushes')]
    public function test_invalid_pushes_are_refused_with_the_vectors_code(object $bad): void
    {
        $outbox = new OutboxManager(new RecordingDriver(), 'cli_1');
        try {
            $outbox->createPush(self::build($bad));
            $this->fail("expected {$bad->code}");
        } catch (PushException $e) {
            $this->assertSame($bad->code, $e->errorCode());
        }
    }

    public function test_the_pinned_vector_is_the_go_sdks_copy_while_it_sits_in_this_repository(): void
    {
        $goCopy = __DIR__ . '/../../../../../pkg/fcsdk/outbox/testdata/conformance/outbox-push.json';
        if (!is_file($goCopy)) {
            $this->markTestSkipped('SDK mirrored without the Go SDK');
        }
        $this->assertSame(file_get_contents($goCopy), file_get_contents(__DIR__ . '/../../Fixtures/push/outbox-push.json'));
    }

    public function test_a_generated_id_is_a_tsid_and_the_push_id(): void
    {
        $driver = new RecordingDriver();
        $id = (new OutboxManager($driver, 'cli_1'))->createPush(CreatePushDto::create('shop:board', 'refresh', []));
        $this->assertSame(13, strlen($id));
        $this->assertSame($id, $driver->rows[0]['id']);
        $this->assertSame($id, json_decode($driver->rows[0]['payload'])->pushId);
        $this->assertSame('{"topic":"shop:board","type":"refresh","data":{},"pushId":"' . $id . '"}', $driver->rows[0]['payload']);
    }

    public function test_data_may_be_json_text_whitespace_is_dropped_and_numbers_kept(): void
    {
        $driver = new RecordingDriver();
        (new OutboxManager($driver, 'cli_1'))->createPush(
            CreatePushDto::create('shop:board', 'refresh', ' { "n" : 1.0 , "s" : "a b" } ')->withId('p1'),
        );
        $this->assertSame(
            '{"topic":"shop:board","type":"refresh","data":{"n":1.0,"s":"a b"},"pushId":"p1"}',
            $driver->rows[0]['payload'],
        );
    }

    public function test_an_application_data_limit_is_honoured_capped_at_64_kib(): void
    {
        $outbox = new OutboxManager(new RecordingDriver(), 'cli_1');
        $mid = CreatePushDto::create('shop:board', 'refresh', ['blob' => str_repeat('x', 20000)]);
        try {
            $outbox->createPush($mid);
            $this->fail('over the default limit');
        } catch (PushException $e) {
            $this->assertSame('DATA_TOO_LARGE', $e->errorCode());
        }
        $outbox->createPush($mid->withMaxDataBytes(32768));
        $this->expectException(PushException::class);
        $outbox->createPush(CreatePushDto::create('shop:board', 'refresh', ['blob' => str_repeat('x', 70000)])->withMaxDataBytes(1_000_000));
    }

    public function test_create_pushes_validates_every_push_before_writing_any(): void
    {
        $driver = new RecordingDriver();
        try {
            (new OutboxManager($driver, 'cli_1'))->createPushes([
                CreatePushDto::create('shop:a', 'refresh', []),
                CreatePushDto::create('Shop:a', 'refresh', []),
            ]);
            $this->fail('expected a refusal');
        } catch (PushException) {
        }
        $this->assertSame([], $driver->rows);
    }

    public function test_a_push_client_code_wins_over_the_writers(): void
    {
        $driver = new RecordingDriver();
        (new OutboxManager($driver, 'cli_1', 'acme'))->createPush(CreatePushDto::create('shop:a', 'refresh', [])->withClientCode('globex'));
        $this->assertSame('globex', json_decode($driver->rows[0]['payload'])->clientCode);
    }
}

final class RecordingDriver implements OutboxDriver
{
    /** @var list<array<string, mixed>> */
    public array $rows = [];

    public function insert(array $message): void
    {
        $this->rows[] = $message;
    }

    public function insertBatch(array $messages): void
    {
        array_push($this->rows, ...$messages);
    }
}
