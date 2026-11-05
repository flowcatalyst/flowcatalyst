<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Unit\Client\Resources;

use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\Client\Resources\DispatchJobs;
use FlowCatalyst\DTOs\Responses\DispatchJobResult;
use FlowCatalyst\Exceptions\FlowCatalystException;
use FlowCatalyst\Outbox\DTOs\CreateDispatchJobDto;
use PHPUnit\Framework\TestCase;

/**
 * `dispatchJobs()` posts to POST /api/dispatch-jobs/batch with each job's
 * batch item (the same mapping the outbox writes) and hands back the
 * platform's per-item results. Requests are captured at
 * FlowCatalystClient::request(); nothing touches the network.
 */
final class DispatchJobsTest extends TestCase
{
    /** @var list<array{0: string, 1: string, 2: array}> */
    private array $calls = [];

    private function client(array $answer): FlowCatalystClient
    {
        $client = $this->getMockBuilder(FlowCatalystClient::class)
            ->disableOriginalConstructor()
            ->onlyMethods(['request'])
            ->getMock();
        $client->method('request')->willReturnCallback(
            function (string $method, string $endpoint, array $options = []) use ($answer) {
                $this->calls[] = [$method, $endpoint, $options];
                return $answer;
            }
        );

        return $client;
    }

    private function job(string $key): CreateDispatchJobDto
    {
        return CreateDispatchJobDto::create(
            source: 'integral',
            code: 'integral:billing:invoice:sync',
            targetUrl: 'https://integral.test/hook',
            payload: ['invoice' => $key],
            dispatchPoolId: '',
        )->withIdempotencyKey($key)->withConnectionId('con_1')->withClientCode('acme')
            ->withMetadata(['invoice' => $key]);
    }

    public function test_dispatch_jobs_is_exposed_on_the_client(): void
    {
        $client = $this->client([]);
        $this->assertInstanceOf(DispatchJobs::class, $client->dispatchJobs());
        $this->assertSame($client->dispatchJobs(), $client->dispatchJobs());
    }

    public function test_batch_posts_the_batch_items_and_returns_the_results_in_order(): void
    {
        $client = $this->client(['results' => [
            ['id' => 'job1', 'status' => 'SUCCESS'],
            ['id' => 'job0', 'status' => 'SUCCESS'],
        ]]);
        $jobs = [$this->job('k1'), $this->job('k2')];

        $results = $client->dispatchJobs()->batch($jobs);

        $this->assertCount(1, $this->calls);
        [$method, $endpoint, $options] = $this->calls[0];
        $this->assertSame('POST', $method);
        $this->assertSame('/api/dispatch-jobs/batch', $endpoint);
        $this->assertSame(['items' => [$jobs[0]->toBatchItem(), $jobs[1]->toBatchItem()]], $options['json']);
        $this->assertSame([['key' => 'invoice', 'value' => 'k1']], $options['json']['items'][0]['metadata']);
        $this->assertSame('con_1', $options['json']['items'][0]['connectionId']);
        $this->assertSame('k2', $options['json']['items'][1]['idempotencyKey']);

        $this->assertContainsOnlyInstancesOf(DispatchJobResult::class, $results);
        $this->assertSame(['job1', 'job0'], array_map(fn(DispatchJobResult $r) => $r->id, $results));
        $this->assertTrue($results[0]->isSuccess());
        $this->assertNull($results[0]->error);
    }

    public function test_batch_hands_back_a_failed_item_as_the_platform_answered_it(): void
    {
        $client = $this->client(['results' => [['id' => '', 'status' => 'BAD_REQUEST', 'error' => 'nope']]]);
        $result = $client->dispatchJobs()->batch([$this->job('k1')])[0];
        $this->assertFalse($result->isSuccess());
        $this->assertSame('BAD_REQUEST', $result->status);
        $this->assertSame('nope', $result->error);
    }

    public function test_an_empty_batch_makes_no_call(): void
    {
        $client = $this->client([]);
        $this->assertSame([], $client->dispatchJobs()->batch([]));
        $this->assertSame([], $this->calls);
    }

    public function test_a_result_count_mismatch_is_an_error(): void
    {
        $client = $this->client(['results' => [['id' => 'j', 'status' => 'SUCCESS']]]);
        $this->expectException(FlowCatalystException::class);
        $client->dispatchJobs()->batch([$this->job('k1'), $this->job('k2')]);
    }

    public function test_create_returns_the_id(): void
    {
        $client = $this->client(['results' => [['id' => 'job_first', 'status' => 'SUCCESS']]]);
        $this->assertSame('job_first', $client->dispatchJobs()->create($this->job('k1')));
        $this->assertCount(1, $this->calls[0][2]['json']['items']);
    }

    public function test_create_throws_on_the_items_error(): void
    {
        $client = $this->client(['results' => [['id' => '', 'status' => 'BAD_REQUEST', 'error' => 'bad target']]]);
        try {
            $client->dispatchJobs()->create($this->job('k1'));
            $this->fail('expected an exception');
        } catch (FlowCatalystException $e) {
            $this->assertStringContainsString('BAD_REQUEST', $e->getMessage());
            $this->assertStringContainsString('bad target', $e->getMessage());
        }
    }

    public function test_over_the_batch_limit_is_refused_before_any_call(): void
    {
        $client = $this->client([]);
        $this->expectException(\InvalidArgumentException::class);
        try {
            $client->dispatchJobs()->batch(array_fill(0, DispatchJobs::MAX_BATCH + 1, $this->job('k')));
        } finally {
            $this->assertSame([], $this->calls);
        }
    }
}
