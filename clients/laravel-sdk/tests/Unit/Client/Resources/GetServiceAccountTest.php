<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Unit\Client\Resources;

use FlowCatalyst\Client\Auth\TokenProviderInterface;
use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\Exceptions\FlowCatalystException;
use GuzzleHttp\Client;
use GuzzleHttp\Handler\MockHandler;
use GuzzleHttp\HandlerStack;
use GuzzleHttp\Middleware;
use GuzzleHttp\Psr7\Response;
use PHPUnit\Framework\TestCase;

/**
 * getServiceAccount reads GET /api/applications/{id}/service-account, which
 * answers the body of GET /api/service-accounts/{id} (docs/wire-contract.md,
 * "Reading an application's service account").
 */
final class GetServiceAccountTest extends TestCase
{
    private \ArrayObject $history;

    private function client(Response $response): FlowCatalystClient
    {
        $this->history = new \ArrayObject();
        $provider = new class implements TokenProviderInterface {
            public function getAccessToken(): string { return 'tok'; }
            public function refreshToken(): string { return 'tok'; }
        };
        $client = new FlowCatalystClient(tokenProvider: $provider, baseUrl: 'https://fc.test', retryAttempts: 1, retryDelay: 1);
        $stack = HandlerStack::create(new MockHandler([$response]));
        $stack->push(Middleware::history($this->history));
        $prop = new \ReflectionProperty(FlowCatalystClient::class, 'httpClient');
        $prop->setValue($client, new Client(['handler' => $stack, 'http_errors' => false, 'base_uri' => 'https://fc.test']));

        return $client;
    }

    public function test_it_reads_the_route_and_maps_the_account(): void
    {
        $client = $this->client(new Response(200, ['Content-Type' => 'application/json'], json_encode([
            'id' => 'sac_1',
            'code' => 'app:orders',
            'name' => 'Orders Service Account',
            'active' => true,
            'clientIds' => [],
            'applicationId' => 'app_1',
            'authType' => 'BEARER_TOKEN',
            'roles' => ['platform:application-service'],
            'principalId' => 'prn_1',
            'oauthClientId' => 'orders-client',
            'createdAt' => '2026-11-18T10:00:00Z',
            'updatedAt' => '2026-11-18T10:00:00Z',
        ])));

        $sa = $client->admin()->applications()->getServiceAccount('app_1');

        $this->assertCount(1, $this->history);
        $request = iterator_to_array($this->history)[0]['request'];
        $this->assertSame('GET', $request->getMethod());
        $this->assertSame('/api/applications/app_1/service-account', $request->getUri()->getPath());
        $this->assertSame('sac_1', $sa->id);
        $this->assertSame('app:orders', $sa->code);
        $this->assertSame('Orders Service Account', $sa->name);
        $this->assertSame('prn_1', $sa->principalId);
        $this->assertSame('orders-client', $sa->oauthClientId);
        $this->assertSame('BEARER_TOKEN', $sa->authType);
        $this->assertSame(['platform:application-service'], $sa->roles);
        $this->assertSame('app_1', $sa->applicationId);
    }

    public function test_an_application_without_an_account_is_a_404(): void
    {
        $client = $this->client(new Response(404, ['Content-Type' => 'application/json'], json_encode([
            'status' => 404,
            'detail' => 'Application app_2 has no service account',
        ])));

        try {
            $client->admin()->applications()->getServiceAccount('app_2');
            $this->fail('expected a 404');
        } catch (FlowCatalystException $e) {
            $this->assertSame(404, $e->getCode());
        }
        $this->assertCount(1, $this->history);
    }
}
