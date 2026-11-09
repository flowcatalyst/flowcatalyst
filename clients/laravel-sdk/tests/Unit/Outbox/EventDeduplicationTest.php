<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Unit\Outbox;

use FlowCatalyst\Outbox\Contracts\OutboxDriver;
use FlowCatalyst\Outbox\DTOs\CreateEventDto;
use FlowCatalyst\Outbox\OutboxManager;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

/**
 * An event's deduplicationId defaults to the outbox row's id, so a batch the outbox processor
 * sends again does not create a second event; an explicit value wins. The EVENT rows of the
 * shared vector (the Go SDK's outbox.json, read in place) carry it: the event's id.
 *
 * Payload bytes are compared after a decode: PHP's json_encode escapes "/" and non-ASCII by
 * default, which the platform reads as the same value.
 */
final class EventDeduplicationTest extends TestCase
{
    private const VECTOR = __DIR__ . '/../../../../../pkg/fcsdk/outbox/testdata/conformance/outbox.json';

    /** @return array<string, array{object}> */
    public static function cases(): array
    {
        if (!is_file(self::VECTOR)) {
            return [];
        }
        $out = [];
        foreach (json_decode((string) file_get_contents(self::VECTOR), false, 512, JSON_THROW_ON_ERROR)->cases as $case) {
            $out[$case->name] = [$case];
        }
        return $out;
    }

    private static function build(object $ev, object $ctx): CreateEventDto
    {
        $seg = explode(':', $ev->type);
        $data = [];
        foreach ((array) $ev->fields as $k => $v) {
            if ($v !== null) {
                $data[$k] = json_decode(json_encode($v), true);
            }
        }
        $dto = CreateEventDto::create($ev->type, $data)
            ->withSource($seg[0] . ':' . $seg[1])
            ->withSubject($seg[1] . '.' . $seg[2] . '.' . $ev->subjectId)
            ->withMessageGroup($ev->messageGroup ?? ($seg[1] . ':' . $seg[2] . ':' . $ev->subjectId));
        if (!empty($ctx->correlation)) {
            $dto = $dto->withCorrelationId($ctx->correlation);
        }
        if (!empty($ctx->causation)) {
            $dto = $dto->withCausationId($ctx->causation);
        }
        if (!empty($ctx->client)) {
            $dto = $dto->withClientCode($ctx->client);
        }
        $cd = [];
        if (!empty($ctx->principal)) {
            $cd[] = ['key' => 'principalId', 'value' => $ctx->principal];
        }
        if (!empty($ctx->execution)) {
            $cd[] = ['key' => 'executionId', 'value' => $ctx->execution];
        }
        return $cd === [] ? $dto : $dto->withContextData($cd);
    }

    /** The payload with both JSON layers (the member and its embedded `data` string) re-encoded alike. */
    private static function normalised(string $payload): string
    {
        $p = json_decode($payload, true, 512, JSON_THROW_ON_ERROR);
        $p['data'] = json_encode(json_decode($p['data'], true, 512, JSON_THROW_ON_ERROR));
        return json_encode($p);
    }

    #[DataProvider('cases')]
    public function test_the_event_rows_are_the_vectors(object $case): void
    {
        $want = array_values(array_filter($case->rows, static fn($r) => $r->type === 'EVENT'));
        $this->assertCount(count($case->events), $want);
        foreach ($case->events as $i => $ev) {
            // The vector pins the row id as the event id; the default dedup id is that row id.
            $got = self::build($ev, $case->context)->toPayload($ev->eventId);
            $this->assertSame($ev->eventId, $got['deduplicationId']);
            $this->assertSame(self::normalised($want[$i]->payloadText), self::normalised(json_encode($got)));
            // Member order is the vector's.
            $this->assertSame(array_keys((array) $want[$i]->payload), array_keys($got));

            $explicit = self::build($ev, $case->context)->withDeduplicationId($ev->eventId)->toPayload('another-row');
            $this->assertSame($got, $explicit);
        }
    }

    public function test_the_deduplication_id_is_the_row_id_and_stable_across_a_resend(): void
    {
        $driver = new DedupRecordingDriver();
        $outbox = new OutboxManager($driver, 'cli_1');
        $event = CreateEventDto::create('shop:sales:order:placed', ['a' => 1]);
        $ids = [$outbox->createEvent($event)];
        array_push($ids, ...$outbox->createEvents([$event, $event]));

        $this->assertCount(3, $driver->rows);
        $this->assertCount(3, array_unique($ids), 'identical events are separate rows');
        foreach ($driver->rows as $i => $row) {
            $this->assertSame($ids[$i], $row['id']);
            // The stored payload is what a resend reads: the same deduplicationId every time.
            $this->assertSame($row['id'], json_decode($row['payload'])->deduplicationId);
        }
    }

    public function test_an_explicit_deduplication_id_wins_and_a_blank_one_does_not(): void
    {
        $driver = new DedupRecordingDriver();
        $outbox = new OutboxManager($driver, 'cli_1');
        $base = CreateEventDto::create('shop:sales:order:placed', []);
        $outbox->createEvent($base->withDeduplicationId('mine'));
        $outbox->createEvent($base->withDeduplicationId('  '));
        $this->assertSame('mine', json_decode($driver->rows[0]['payload'])->deduplicationId);
        $this->assertSame($driver->rows[1]['id'], json_decode($driver->rows[1]['payload'])->deduplicationId);
    }
}

final class DedupRecordingDriver implements OutboxDriver
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
