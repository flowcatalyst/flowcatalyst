<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Unit\Client\Resources;

use FlowCatalyst\Client\Auth\TokenProviderInterface;
use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\Generated\Client as GeneratedClient;
use FlowCatalyst\Exceptions\FlowCatalystException;
use FlowCatalyst\Generated\Model\PutFieldPolicyRequest;
use GuzzleHttp\Client;
use GuzzleHttp\Handler\MockHandler;
use GuzzleHttp\HandlerStack;
use GuzzleHttp\Middleware;
use GuzzleHttp\Psr7\Response;
use PHPUnit\Framework\TestCase;

/**
 * The photo calls: the generated client sends the image as a byte stream but drops
 * an image answer (and a 304), so getPhoto reads the response itself.
 */
final class FieldPhotosTest extends TestCase
{
    private \ArrayObject $history;

    /** @param list<Response> $responses */
    private function client(array $responses): FlowCatalystClient
    {
        $this->history = new \ArrayObject();
        $stack = HandlerStack::create(new MockHandler($responses));
        $stack->push(Middleware::history($this->history));
        $generated = GeneratedClient::create(new Client(['handler' => $stack, 'http_errors' => false, 'base_uri' => 'https://fc.test']));
        $provider = new class implements TokenProviderInterface {
            public function getAccessToken(): string { return 'tok'; }
            public function refreshToken(): string { return 'tok'; }
        };

        return new class($provider, $generated) extends FlowCatalystClient {
            public function __construct(TokenProviderInterface $p, private readonly GeneratedClient $g)
            {
                parent::__construct(tokenProvider: $p, baseUrl: 'https://fc.test');
            }

            public function generated(): GeneratedClient
            {
                return $this->g;
            }
        };
    }

    private function json(array $body, int $status = 200): Response
    {
        return new Response($status, ['Content-Type' => 'application/json'], json_encode($body));
    }

    private function nothing(): Response
    {
        return $this->json(['id' => 'x', 'status' => 'ok']);
    }

    /** @return array{0: string, 1: string, 2: string, 3: string} method, path, query, body */
    private function seen(int $i = 0): array
    {
        $r = $this->history[$i]['request'];
        return [$r->getMethod(), $r->getUri()->getPath(), $r->getUri()->getQuery(), (string) $r->getBody()];
    }

    private function worker(): array
    {
        return ['id' => 'fwu_1', 'clientId' => 'clt_1', 'workerCode' => 'W1', 'name' => 'Ann', 'photo' => ['version' => '0123456789abcdef', 'updatedAt' => '2026-11-20T10:00:00Z']];
    }

    private const JPEG = "\xFF\xD8\xFF\xE0\x00\x10JFIF\x00\xFF\xD9";

    public function test_set_photo_sends_the_bytes_as_they_are(): void
    {
        $c = $this->client([$this->json($this->worker())]);
        $w = $c->fieldWorkers()->setPhoto('fwu_1', self::JPEG);
        [$method, $path, , $body] = $this->seen(0);
        $this->assertSame('PUT', $method);
        $this->assertSame('/api/field-workers/fwu_1/photo', $path);
        $this->assertSame(self::JPEG, $body, 'not JSON-encoded');
        $this->assertSame('application/octet-stream', $this->history[0]['request']->getHeaderLine('Content-Type'));
        $this->assertSame('0123456789abcdef', $w?->getPhoto()?->getVersion());
    }

    public function test_get_photo_returns_the_bytes_and_the_validator(): void
    {
        $c = $this->client([
            new Response(200, ['Content-Type' => 'image/jpeg', 'ETag' => '"0123456789abcdef-thumb"'], self::JPEG),
            new Response(200, ['Content-Type' => 'image/jpeg', 'ETag' => '"0123456789abcdef-full"'], self::JPEG),
        ]);
        $img = $c->fieldWorkers()->getPhoto('fwu_1');
        $this->assertSame(['GET', '/api/field-workers/fwu_1/photo'], array_slice($this->seen(0), 0, 2));
        $this->assertSame('size=thumb', $this->seen(0)[2]);
        $this->assertSame(self::JPEG, $img->bytes, 'bytes, not a decoded body');
        $this->assertSame('image/jpeg', $img->contentType);
        $this->assertSame('"0123456789abcdef-thumb"', $img->etag);
        $this->assertFalse($img->notModified);
        $c->fieldWorkers()->getPhoto('fwu_1', 'full');
        $this->assertSame('size=full', $this->seen(1)[2]);
    }

    public function test_get_photo_not_modified_is_not_an_error(): void
    {
        $c = $this->client([new Response(304, ['ETag' => '"0123456789abcdef-thumb"'])]);
        $img = $c->fieldWorkers()->getPhoto('fwu_1', 'thumb', '"0123456789abcdef-thumb"');
        $this->assertSame('"0123456789abcdef-thumb"', $this->history[0]['request']->getHeaderLine('If-None-Match'));
        $this->assertTrue($img->notModified);
        $this->assertNull($img->bytes);
        $this->assertSame('"0123456789abcdef-thumb"', $img->etag);
    }

    public function test_get_photo_of_a_worker_with_none_throws_with_the_code(): void
    {
        $c = $this->client([$this->json(['error' => 'PHOTO_NOT_FOUND', 'message' => 'this worker has no photo'], 404)]);
        try {
            $c->fieldWorkers()->getPhoto('fwu_1');
            $this->fail('expected an exception');
        } catch (FlowCatalystException $e) {
            $this->assertSame(404, $e->getCode());
            $this->assertSame('PHOTO_NOT_FOUND', $e->getContext()['code']);
            $this->assertSame('this worker has no photo', $e->getMessage());
        }
    }

    public function test_remove_photo_and_delete_all_photos(): void
    {
        $c = $this->client([$this->json($this->worker()), $this->json(['removed' => 3])]);
        $c->fieldWorkers()->removePhoto('fwu_1');
        $this->assertSame(['DELETE', '/api/field-workers/fwu_1/photo'], array_slice($this->seen(0), 0, 2));
        $r = $c->fieldPolicies()->deletePhotos('clt_1');
        $this->assertSame(['DELETE', '/api/field-policies/clt_1/photos'], array_slice($this->seen(1), 0, 2));
        $this->assertSame(3, $r?->getRemoved());
    }

    public function test_the_policy_carries_the_two_photo_settings(): void
    {
        $c = $this->client([$this->json(['clientId' => 'clt_1', 'photosEnabled' => true, 'workerMaySetOwnPhoto' => false]), $this->json(['clientId' => 'clt_1'])]);
        $p = $c->fieldPolicies()->get('clt_1');
        $this->assertTrue($p?->getPhotosEnabled());
        $this->assertFalse($p?->getWorkerMaySetOwnPhoto());
        $c->fieldPolicies()->put('clt_1', (new PutFieldPolicyRequest())->setRequireAttestation(false)->setPhotosEnabled(false)->setWorkerMaySetOwnPhoto(true));
        $this->assertEquals(['requireAttestation' => false, 'photosEnabled' => false, 'workerMaySetOwnPhoto' => true], json_decode($this->seen(1)[3], true));
    }
}
