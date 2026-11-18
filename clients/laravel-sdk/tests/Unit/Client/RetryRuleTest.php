<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Unit\Client;

use FlowCatalyst\Client\Auth\TokenProviderInterface;
use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\DTOs\Requests\CreateApplicationRequest;
use FlowCatalyst\DTOs\Requests\UpdateApplicationRequest;
use FlowCatalyst\DTOs\Requests\SyncRoleEntry;
use FlowCatalyst\Exceptions\FlowCatalystException;
use FlowCatalyst\Exceptions\OutcomeUnknownException;
use FlowCatalyst\Outbox\DTOs\CreateDispatchJobDto;
use GuzzleHttp\Client;
use GuzzleHttp\Exception\ConnectException;
use GuzzleHttp\Exception\RequestException;
use GuzzleHttp\Handler\MockHandler;
use GuzzleHttp\HandlerStack;
use GuzzleHttp\Middleware;
use GuzzleHttp\Psr7\Request;
use GuzzleHttp\Psr7\Response;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

/**
 * The retry rule of FlowCatalystClient::request(): a request is sent again
 * only when that is safe. GET/HEAD/OPTIONS/PUT/DELETE and the POSTs a resource
 * marks repeatable (the declarative syncs, a dispatch-job batch whose every
 * job has an idempotency key) are retried on 502/503/504 and on a transport
 * failure after sending; any other POST is not, and throws
 * OutcomeUnknownException. 408/429 and a connection never made are retried
 * for every request.
 */
final class RetryRuleTest extends TestCase
{
    private \ArrayObject $history;

    /** @param list<Response|\Throwable> $responses */
    private function client(array $responses, int $retryAttempts = 3): FlowCatalystClient
    {
        $this->history = new \ArrayObject();
        $client = new FlowCatalystClient(
            tokenProvider: new class implements TokenProviderInterface {
                public function getAccessToken(): string { return 'tok'; }
                public function refreshToken(): string { return 'tok'; }
            },
            baseUrl: 'https://fc.test',
            retryAttempts: $retryAttempts,
            retryDelay: 1,
        );
        $stack = HandlerStack::create(new MockHandler($responses));
        $stack->push(Middleware::history($this->history));
        $prop = new \ReflectionProperty(FlowCatalystClient::class, 'httpClient');
        $prop->setValue($client, new Client(['handler' => $stack, 'http_errors' => false, 'base_uri' => 'https://fc.test']));

        return $client;
    }

    private static function json(int $status, array $body = []): Response
    {
        return new Response($status, ['Content-Type' => 'application/json'], json_encode($body));
    }

    private static function app(): Response
    {
        return self::json(200, [
            'id' => 'app_1', 'code' => 'orders', 'name' => 'Orders', 'type' => 'APPLICATION', 'active' => true,
            'createdAt' => '2026-11-18T10:00:00Z', 'updatedAt' => '2026-11-18T10:00:00Z',
        ]);
    }

    /** cURL's report of a request it wrote and got no answer to (errno 52, empty reply). */
    private static function cutOffAfterSending(string $method = 'POST'): RequestException
    {
        return new RequestException('cURL error 52: Empty reply from server', new Request($method, 'https://fc.test/x'),
            null, null, ['errno' => 52, 'request_size' => 180]);
    }

    /** cURL's report of a timeout after the request was written (errno 28). */
    private static function timedOutAfterSending(string $method = 'POST'): ConnectException
    {
        return new ConnectException('cURL error 28: Operation timed out', new Request($method, 'https://fc.test/x'),
            null, ['errno' => 28, 'request_size' => 180]);
    }

    /** cURL's report of a refused connection (errno 7): nothing was sent. */
    private static function refused(string $method = 'POST'): ConnectException
    {
        return new ConnectException('cURL error 7: Failed to connect', new Request($method, 'https://fc.test/x'),
            null, ['errno' => 7, 'request_size' => 0]);
    }

    private function requests(): array
    {
        return array_map(
            static fn(array $e) => $e['request']->getMethod() . ' ' . $e['request']->getUri()->getPath(),
            iterator_to_array($this->history),
        );
    }

    public function test_a_get_answered_502_is_retried(): void
    {
        $client = $this->client([self::json(502), self::app()]);
        $this->assertSame('app_1', $client->admin()->applications()->get('app_1')->id);
        $this->assertCount(2, $this->history);
    }

    public function test_a_put_answered_503_is_retried(): void
    {
        $client = $this->client([self::json(503), self::app()]);
        $client->admin()->applications()->update('app_1', new UpdateApplicationRequest(name: 'Orders'));
        $this->assertSame(['PUT /api/applications/app_1', 'PUT /api/applications/app_1'], $this->requests());
    }

    public function test_a_delete_answered_502_then_404_has_succeeded(): void
    {
        $client = $this->client([self::json(502), self::json(404, ['message' => 'not found'])]);
        $this->assertSame([], $client->request('DELETE', '/api/applications/app_1'));
        $this->assertCount(2, $this->history);
    }

    public function test_a_delete_cut_off_after_sending_then_404_has_succeeded(): void
    {
        $client = $this->client([self::cutOffAfterSending('DELETE'), self::json(404, ['message' => 'not found'])]);
        $this->assertSame([], $client->request('DELETE', '/api/applications/app_1'));
        $this->assertCount(2, $this->history);
    }

    public function test_a_delete_answered_404_first_is_not_found(): void
    {
        $client = $this->client([self::json(404, ['message' => 'not found'])]);
        try {
            $client->request('DELETE', '/api/applications/app_1');
            $this->fail('expected a 404');
        } catch (FlowCatalystException $e) {
            $this->assertNotInstanceOf(OutcomeUnknownException::class, $e);
            $this->assertSame(404, $e->getCode());
        }
        $this->assertCount(1, $this->history);
    }

    public function test_a_delete_answered_429_then_404_is_not_found(): void
    {
        $client = $this->client([self::json(429), self::json(404, ['message' => 'not found'])]);
        $this->expectExceptionCode(404);
        try {
            $client->request('DELETE', '/api/applications/app_1');
        } finally {
            $this->assertCount(2, $this->history);
        }
    }

    /** @return array<string, array{int}> */
    public static function gatewayStatuses(): array
    {
        return ['502' => [502], '503' => [503], '504' => [504]];
    }

    #[DataProvider('gatewayStatuses')]
    public function test_a_post_answered_a_gateway_status_is_not_repeated_and_its_outcome_is_unknown(int $status): void
    {
        $client = $this->client([self::json($status, ['message' => "gateway {$status}"]), self::json(201, ['id' => 'app_2'])]);
        try {
            $client->admin()->applications()->create(new CreateApplicationRequest(code: 'x', name: 'X'));
            $this->fail('expected OutcomeUnknownException');
        } catch (OutcomeUnknownException $e) {
            $this->assertInstanceOf(FlowCatalystException::class, $e);
            $this->assertSame($status, $e->status);
            $this->assertSame($status, $e->getCode());
            $this->assertSame('POST', $e->method);
            $this->assertSame('/api/applications', $e->endpoint);
            $this->assertSame(['message' => "gateway {$status}"], $e->getContext());
            $this->assertStringContainsString('may have been applied and was not repeated', $e->getMessage());
        }
        $this->assertCount(1, $this->history);
    }

    /**
     * As reported: the platform committed the provisioning and the gateway
     * lost its answer. A second POST would be refused 409 and the one-time
     * secrets lost, so it is never sent.
     */
    public function test_provisioning_answered_502_is_sent_once_and_the_scripted_409_is_never_requested(): void
    {
        $client = $this->client([
            self::json(502, ['message' => 'bad gateway']),
            self::json(409, ['error' => 'ALREADY_PROVISIONED', 'message' => 'already provisioned']),
        ]);
        try {
            $client->admin()->applications()->provisionServiceAccount('app_1');
            $this->fail('expected OutcomeUnknownException');
        } catch (OutcomeUnknownException $e) {
            $this->assertSame(502, $e->status);
        }
        $this->assertSame(['POST /api/applications/app_1/provision-service-account'], $this->requests());
    }

    public function test_a_post_answered_500_is_the_ordinary_error_not_repeated(): void
    {
        $client = $this->client([self::json(500, ['message' => 'boom']), self::json(200)]);
        try {
            $client->request('POST', '/api/applications/app_1/activate');
            $this->fail('expected a 500');
        } catch (FlowCatalystException $e) {
            $this->assertNotInstanceOf(OutcomeUnknownException::class, $e);
            $this->assertSame(500, $e->getCode());
        }
        $this->assertCount(1, $this->history);
    }

    public function test_a_post_to_a_declarative_sync_answered_502_is_retried(): void
    {
        $client = $this->client([
            self::json(502),
            self::json(200, ['applicationCode' => 'orders', 'created' => 1, 'updated' => 0, 'deleted' => 0, 'syncedCodes' => ['orders:clerk']]),
        ]);
        $client->roles()->sync('orders', [new SyncRoleEntry(name: 'clerk')]);
        $this->assertSame(['POST /api/applications/orders/roles/sync', 'POST /api/applications/orders/roles/sync'], $this->requests());
    }

    public function test_a_dispatch_job_batch_is_retried_only_when_every_job_has_an_idempotency_key(): void
    {
        $job = fn(?string $key) => new CreateDispatchJobDto(
            source: 'orders', code: 'orders:sales:order:ping', targetUrl: 'https://orders.test/hook', payload: '{}',
            dispatchPoolId: 'dpl_1', idempotencyKey: $key,
        );
        $answer = self::json(201, ['results' => [['id' => 'dsp_1', 'status' => 'SUCCESS'], ['id' => 'dsp_2', 'status' => 'SUCCESS']]]);

        $client = $this->client([self::json(502), $answer]);
        $client->dispatchJobs()->batch([$job('k1'), $job('k2')]);
        $this->assertCount(2, $this->history, 'every job keyed: sent again');

        $client = $this->client([self::json(502), $answer]);
        $this->expectException(OutcomeUnknownException::class);
        try {
            $client->dispatchJobs()->batch([$job('k1'), $job(null)]);
        } finally {
            $this->assertCount(1, $this->history, 'a job without a key: not sent again');
        }
    }

    /** @return array<string, array{int}> */
    public static function refusals(): array
    {
        return ['408' => [408], '429' => [429]];
    }

    #[DataProvider('refusals')]
    public function test_a_post_refused_before_processing_is_retried(int $status): void
    {
        $client = $this->client([self::json($status), self::json(201, ['id' => 'app_2'])]);
        $this->assertSame(['id' => 'app_2'], $client->request('POST', '/api/applications'));
        $this->assertCount(2, $this->history);
    }

    public function test_a_post_that_could_not_connect_is_retried(): void
    {
        $client = $this->client([self::refused(), self::json(201, ['id' => 'app_2'])]);
        $this->assertSame(['id' => 'app_2'], $client->request('POST', '/api/applications'));
        $this->assertCount(2, $this->history);
    }

    public function test_a_post_that_can_never_connect_is_a_transport_error_not_an_unknown_outcome(): void
    {
        $client = $this->client([self::refused(), self::refused()], retryAttempts: 2);
        try {
            $client->request('POST', '/api/applications');
            $this->fail('expected a transport error');
        } catch (FlowCatalystException $e) {
            $this->assertNotInstanceOf(OutcomeUnknownException::class, $e);
            $this->assertStringContainsString('Failed to connect', $e->getMessage());
        }
        $this->assertCount(2, $this->history);
    }

    public function test_a_post_cut_off_after_sending_is_not_repeated_and_its_outcome_is_unknown(): void
    {
        $client = $this->client([self::cutOffAfterSending(), self::json(201, ['id' => 'app_2'])]);
        try {
            $client->request('POST', '/api/applications');
            $this->fail('expected OutcomeUnknownException');
        } catch (OutcomeUnknownException $e) {
            $this->assertNull($e->status);
            $this->assertSame(0, $e->getCode());
            $this->assertInstanceOf(FlowCatalystException::class, $e->getPrevious());
            $this->assertStringContainsString('Empty reply', $e->getMessage());
        }
        $this->assertCount(1, $this->history);
    }

    public function test_a_post_that_timed_out_after_sending_is_not_repeated_and_its_outcome_is_unknown(): void
    {
        $client = $this->client([self::timedOutAfterSending(), self::json(201, ['id' => 'app_2'])]);
        $this->expectException(OutcomeUnknownException::class);
        try {
            $client->request('POST', '/api/applications');
        } finally {
            $this->assertCount(1, $this->history);
        }
    }

    public function test_a_post_whose_connect_timed_out_before_sending_is_retried(): void
    {
        $connectTimeout = new ConnectException('cURL error 28: Connection timed out', new Request('POST', 'https://fc.test/x'),
            null, ['errno' => 28, 'request_size' => 0]);
        $client = $this->client([$connectTimeout, self::json(201, ['id' => 'app_2'])]);
        $this->assertSame(['id' => 'app_2'], $client->request('POST', '/api/applications'));
        $this->assertCount(2, $this->history);
    }

    public function test_a_get_that_timed_out_is_retried(): void
    {
        $client = $this->client([self::timedOutAfterSending('GET'), self::app()]);
        $this->assertSame('app_1', $client->admin()->applications()->get('app_1')->id);
        $this->assertCount(2, $this->history);
    }

    public function test_with_retries_off_a_post_answered_502_is_still_an_unknown_outcome_and_a_get_the_ordinary_error(): void
    {
        $client = $this->client([self::json(502), self::json(502)], retryAttempts: 1);
        try {
            $client->request('POST', '/api/applications/app_1/activate');
            $this->fail('expected OutcomeUnknownException');
        } catch (OutcomeUnknownException) {
        }
        try {
            $client->request('GET', '/api/applications/app_1');
            $this->fail('expected a 502');
        } catch (FlowCatalystException $e) {
            $this->assertNotInstanceOf(OutcomeUnknownException::class, $e);
            $this->assertSame(502, $e->getCode());
        }
        $this->assertCount(2, $this->history);
    }

    public function test_a_custom_post_marked_repeatable_is_retried(): void
    {
        $client = $this->client([self::json(503), self::json(200, ['ok' => true])]);
        $this->assertSame(['ok' => true], $client->request('POST', '/api/principals/match-emails', ['json' => []], repeatable: true));
        $this->assertCount(2, $this->history);
    }
}
