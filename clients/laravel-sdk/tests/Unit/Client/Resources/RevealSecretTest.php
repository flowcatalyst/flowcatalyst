<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Unit\Client\Resources;

use FlowCatalyst\Client\Auth\TokenProviderInterface;
use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\DTOs\PlatformConfigSecret;
use FlowCatalyst\Exceptions\FlowCatalystException;
use GuzzleHttp\Client;
use GuzzleHttp\Handler\MockHandler;
use GuzzleHttp\HandlerStack;
use GuzzleHttp\Middleware;
use GuzzleHttp\Psr7\Response;
use PHPUnit\Framework\TestCase;

final class RevealSecretTest extends TestCase
{
    private const SECRET = 'hunter2-PLAINTEXT';

    private \ArrayObject $history;

    /** @param array<int, Response> $responses */
    private function client(array $responses): FlowCatalystClient
    {
        $this->history = new \ArrayObject();
        $provider = new class implements TokenProviderInterface {
            public function getAccessToken(): string { return 'tok'; }
            public function refreshToken(): string { return 'tok'; }
        };
        $client = new FlowCatalystClient(tokenProvider: $provider, baseUrl: 'https://fc.test', retryAttempts: 1, retryDelay: 1);
        $stack = HandlerStack::create(new MockHandler($responses));
        $stack->push(Middleware::history($this->history));
        $prop = new \ReflectionProperty(FlowCatalystClient::class, 'httpClient');
        $prop->setAccessible(true);
        $prop->setValue($client, new Client(['handler' => $stack, 'http_errors' => false, 'base_uri' => 'https://fc.test']));

        return $client;
    }

    private function ok(): Response
    {
        return new Response(200, ['Content-Type' => 'application/json', 'Cache-Control' => 'no-store'], json_encode([
            'applicationCode' => 'rfp', 'section' => 'mail', 'property' => 'pw',
            'value' => self::SECRET, 'updatedAt' => '2026-01-02T00:00:00Z',
        ]));
    }

    private function request(int $i = 0): \Psr\Http\Message\RequestInterface
    {
        return iterator_to_array($this->history)[$i]['request'];
    }

    public function test_success_builds_path_and_omits_client_id(): void
    {
        $s = $this->client([$this->ok()])->platformConfig()->revealSecret('rfp', 'mail', 'pw');

        $this->assertSame('GET', $this->request()->getMethod());
        $this->assertSame('/api/config/rfp/mail/pw/secret', $this->request()->getUri()->getPath());
        $this->assertSame('', $this->request()->getUri()->getQuery());
        $this->assertSame(self::SECRET, $s->value);
        $this->assertSame(self::SECRET, $s->revealedValue());
        $this->assertNull($s->clientId);
    }

    public function test_client_id_query_and_segment_encoding(): void
    {
        $this->client([$this->ok()])->platformConfig()->revealSecret('a b', 's/x', 'p?q', 'clt_1');

        $uri = $this->request()->getUri();
        $this->assertSame('/api/config/a%20b/s%2Fx/p%3Fq/secret', $uri->getPath());
        $this->assertSame('clientId=clt_1', $uri->getQuery());
    }

    public function test_on_behalf_of_goes_out_as_a_header_beside_the_bearer(): void
    {
        $pc = $this->client([$this->ok(), $this->ok()])->platformConfig();
        $pc->revealSecret('rfp', 'mail', 'pw', onBehalfOf: 'usr:alice@example.com');
        $pc->revealSecret('rfp', 'mail', 'pw');

        $this->assertSame('usr:alice@example.com', $this->request(0)->getHeaderLine('X-On-Behalf-Of'));
        $this->assertSame('Bearer tok', $this->request(0)->getHeaderLine('Authorization'));
        $this->assertFalse($this->request(1)->hasHeader('X-On-Behalf-Of'));
    }

    public function test_not_cached_each_call_hits_the_platform(): void
    {
        $pc = $this->client([$this->ok(), $this->ok()])->platformConfig();
        $pc->revealSecret('rfp', 'mail', 'pw');
        $pc->revealSecret('rfp', 'mail', 'pw');

        $this->assertCount(2, $this->history);
        $this->assertFalse($this->request()->hasHeader('Cache-Control'));
    }

    /** @return array<string, array{0: int, 1: string}> */
    public static function errors(): array
    {
        return [
            '403' => [403, 'FORBIDDEN'],
            '404' => [404, 'NOT_FOUND'],
            '400 not a secret' => [400, 'NOT_A_SECRET'],
        ];
    }

    /** @dataProvider errors */
    public function test_error_statuses_map_to_exceptions(int $status, string $code): void
    {
        $c = $this->client([new Response($status, ['Content-Type' => 'application/json'], json_encode(['error' => $code, 'message' => "m $code"]))]);

        try {
            $c->platformConfig()->revealSecret('rfp', 'mail', 'pw');
            $this->fail('expected exception');
        } catch (FlowCatalystException $e) {
            $this->assertSame($status, $e->getCode());
            $this->assertSame("m $code", $e->getMessage());
        }
        $this->assertCount(1, $this->history);
    }

    public function test_value_is_redacted_in_every_stringified_form(): void
    {
        $s = PlatformConfigSecret::fromArray(['applicationCode' => 'rfp', 'section' => 'mail', 'property' => 'pw', 'value' => self::SECRET, 'updatedAt' => 'u']);

        ob_start();
        var_dump($s);
        $forms = [ob_get_clean(), print_r($s, true), json_encode($s), json_encode($s->toArray()), serialize($s)];
        foreach ($forms as $form) {
            $this->assertStringNotContainsString(self::SECRET, $form);
        }
        $this->assertStringContainsString('***', $forms[1]);
        $this->assertSame('***', $s->toArray()['value']);
    }
}
