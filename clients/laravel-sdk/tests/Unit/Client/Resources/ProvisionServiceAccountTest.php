<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Unit\Client\Resources;

use FlowCatalyst\Client\Auth\TokenProviderInterface;
use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\DTOs\ProvisionedServiceAccount;
use GuzzleHttp\Client;
use GuzzleHttp\Handler\MockHandler;
use GuzzleHttp\HandlerStack;
use GuzzleHttp\Middleware;
use GuzzleHttp\Psr7\Response;
use PHPUnit\Framework\TestCase;

/**
 * provisionServiceAccount returns what the platform answers
 * (docs/wire-contract.md, "Application service-account provisioning"),
 * with every one-time secret readable and redacted everywhere else.
 */
final class ProvisionServiceAccountTest extends TestCase
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
        $prop->setAccessible(true);
        $prop->setValue($client, new Client(['handler' => $stack, 'http_errors' => false, 'base_uri' => 'https://fc.test']));

        return $client;
    }

    /** @param array<string, mixed>|null $webhook */
    private function provisioned(?array $webhook): Response
    {
        $sa = [
            'principalId' => 'prn_1',
            'name' => 'Orders Service Account',
            'oauthClient' => ['id' => 'oac_1', 'clientId' => 'cid', 'clientSecret' => 'csecret-PLAIN'],
        ];
        if ($webhook !== null) {
            $sa['webhook'] = $webhook;
        }

        return new Response(201, ['Content-Type' => 'application/json'], json_encode([
            'message' => 'Service account provisioned',
            'serviceAccount' => $sa,
        ]));
    }

    public function test_returns_the_credentials_and_webhook_secrets(): void
    {
        $client = $this->client($this->provisioned(['authToken' => 'fc_token-PLAIN', 'signingSecret' => 'sign-PLAIN']));
        $p = $client->admin()->applications()->provisionServiceAccount('app_1');

        $request = iterator_to_array($this->history)[0]['request'];
        $this->assertSame('POST', $request->getMethod());
        $this->assertSame('/api/applications/app_1/provision-service-account', $request->getUri()->getPath());

        $this->assertInstanceOf(ProvisionedServiceAccount::class, $p);
        $this->assertSame('Service account provisioned', $p->message);
        $this->assertSame('prn_1', $p->principalId);
        $this->assertSame('Orders Service Account', $p->name);
        $this->assertSame('oac_1', $p->oauthClientId);
        $this->assertSame('cid', $p->clientId);
        $this->assertSame('csecret-PLAIN', $p->clientSecret);
        $this->assertSame('fc_token-PLAIN', $p->webhookAuthToken);
        $this->assertSame('sign-PLAIN', $p->webhookSigningSecret);
    }

    public function test_secrets_are_redacted_outside_the_properties(): void
    {
        $p = $this->client($this->provisioned(['authToken' => 'fc_token-PLAIN', 'signingSecret' => 'sign-PLAIN']))
            ->admin()->applications()->provisionServiceAccount('app_1');

        foreach ([json_encode($p), print_r($p, true), var_export($p->toArray(), true), serialize($p)] as $rendered) {
            $this->assertStringNotContainsString('PLAIN', $rendered);
        }
        $this->assertSame([
            'message' => 'Service account provisioned',
            'serviceAccount' => [
                'principalId' => 'prn_1',
                'name' => 'Orders Service Account',
                'oauthClient' => ['id' => 'oac_1', 'clientId' => 'cid', 'clientSecret' => '***'],
                'webhook' => ['authToken' => '***', 'signingSecret' => '***'],
            ],
        ], $p->toArray());
    }

    public function test_a_platform_without_webhook_credentials_gives_null(): void
    {
        $p = $this->client($this->provisioned(null))->admin()->applications()->provisionServiceAccount('app_1');

        $this->assertSame('csecret-PLAIN', $p->clientSecret);
        $this->assertNull($p->webhookAuthToken);
        $this->assertNull($p->webhookSigningSecret);
        $this->assertArrayNotHasKey('webhook', $p->toArray()['serviceAccount']);
    }
}
