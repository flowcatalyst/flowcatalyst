<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Unit\Client\Resources;

use FlowCatalyst\Client\Auth\TokenProviderInterface;
use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\Generated\Client as GeneratedClient;
use FlowCatalyst\Generated\Model\CreateEnrolmentCodeRequest;
use FlowCatalyst\Generated\Model\EnsureFieldWorkerRequest;
use FlowCatalyst\Generated\Model\PutFieldPolicyRequest;
use FlowCatalyst\Generated\Model\UpdateFieldWorkerRequest;
use GuzzleHttp\Client;
use GuzzleHttp\Handler\MockHandler;
use GuzzleHttp\HandlerStack;
use GuzzleHttp\Middleware;
use GuzzleHttp\Psr7\Response;
use PHPUnit\Framework\TestCase;

/**
 * The field admin wrappers go through the generated client: each call must
 * land on the right method + path (HTTP layer mocked with a Guzzle handler).
 */
final class FieldResourcesTest extends TestCase
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

    public function test_workers_hit_the_right_endpoints(): void
    {
        $worker = ['id' => 'fwu_1', 'clientId' => 'clt_1', 'workerCode' => 'W1', 'name' => 'Ann'];
        $c = $this->client([
            $this->json($worker, 201), $this->json(['workers' => [], 'total' => 0, 'page' => 0, 'size' => 100]),
            $this->json($worker), $this->json($worker),
            $this->nothing(), $this->json(['pin' => '123456']), $this->nothing(), $this->nothing(), $this->nothing(),
            $this->nothing(), $this->nothing(), $this->json(['signIns' => [], 'total' => 0, 'page' => 0, 'size' => 100]),
        ]);
        $w = $c->fieldWorkers();

        $w->ensure((new EnsureFieldWorkerRequest())->setClientId('clt_1')->setWorkerCode('W1')->setName('Ann')->setAppCodes(['a']));
        $this->assertSame(['POST', '/api/field-workers'], array_slice($this->seen(0), 0, 2));
        $this->assertEquals(['clientId' => 'clt_1', 'workerCode' => 'W1', 'name' => 'Ann', 'appCodes' => ['a']], json_decode($this->seen(0)[3], true));

        $w->list('clt_1', ['appCode' => 'a', 'page' => 2]);
        $this->assertSame(['GET', '/api/field-workers'], array_slice($this->seen(1), 0, 2));
        parse_str($this->seen(1)[2], $q);
        $this->assertSame(['clientId' => 'clt_1', 'appCode' => 'a', 'page' => '2'], $q);

        $w->get('fwu_1');
        $this->assertSame(['GET', '/api/field-workers/fwu_1'], array_slice($this->seen(2), 0, 2));
        $w->update('fwu_1', (new UpdateFieldWorkerRequest())->setName('Anne'));
        $this->assertSame(['PUT', '/api/field-workers/fwu_1'], array_slice($this->seen(3), 0, 2));
        $w->delete('fwu_1');
        $this->assertSame(['DELETE', '/api/field-workers/fwu_1'], array_slice($this->seen(4), 0, 2));
        $w->resetPin('fwu_1');
        $this->assertSame(['POST', '/api/field-workers/fwu_1/pin-reset'], array_slice($this->seen(5), 0, 2));
        $w->unlock('fwu_1');
        $this->assertSame(['POST', '/api/field-workers/fwu_1/unlock'], array_slice($this->seen(6), 0, 2));
        $w->suspend('fwu_1');
        $this->assertSame(['POST', '/api/field-workers/fwu_1/suspend'], array_slice($this->seen(7), 0, 2));
        $w->activate('fwu_1');
        $this->assertSame(['POST', '/api/field-workers/fwu_1/activate'], array_slice($this->seen(8), 0, 2));
        $w->grantApp('fwu_1', 'depot');
        $this->assertSame(['POST', '/api/field-workers/fwu_1/apps'], array_slice($this->seen(9), 0, 2));
        $this->assertSame(['appCode' => 'depot'], json_decode($this->seen(9)[3], true));
        $w->revokeApp('fwu_1', 'depot');
        $this->assertSame(['DELETE', '/api/field-workers/fwu_1/apps/depot'], array_slice($this->seen(10), 0, 2));
        $w->signIns('fwu_1', 1, 10);
        $this->assertSame(['GET', '/api/field-workers/fwu_1/sign-ins'], array_slice($this->seen(11), 0, 2));
        parse_str($this->seen(11)[2], $q);
        $this->assertSame(['page' => '1', 'pageSize' => '10'], $q);
    }

    public function test_badges_sign_in_codes_and_identity_flags(): void
    {
        $badge = ['id' => 'fbg_1', 'workerId' => 'fwu_1', 'status' => 'ACTIVE', 'createdAt' => '2026-01-01T00:00:00Z'];
        $c = $this->client([
            $this->json(['badge' => $badge, 'token' => 'secret'], 201),
            $this->json(['badges' => [$badge]]),
            $this->json(['status' => 'REVOKED'] + $badge),
            $this->json(['issued' => [['workerId' => 'fwu_1', 'workerCode' => 'W1', 'name' => 'Ann', 'badge' => $badge, 'token' => 't1']]], 201),
            $this->json(['code' => 'ABC', 'workerId' => 'fwu_1', 'appCode' => 'depot', 'expiresAt' => '2026-01-01T00:05:00Z'], 201),
            $this->json(['id' => 'fwu_1', 'clientId' => 'clt_1', 'workerCode' => 'W1', 'name' => 'Ann'], 201),
            $this->json(['workers' => [], 'total' => 0, 'page' => 0, 'size' => 100]),
        ]);
        $w = $c->fieldWorkers();

        $issued = $w->issueBadge('fwu_1', 'Depot A');
        $this->assertSame(['POST', '/api/field-workers/fwu_1/badges'], array_slice($this->seen(0), 0, 2));
        $this->assertSame(['label' => 'Depot A'], json_decode($this->seen(0)[3], true));
        $this->assertSame('secret', $issued?->getToken());

        $w->listBadges('fwu_1');
        $this->assertSame(['GET', '/api/field-workers/fwu_1/badges'], array_slice($this->seen(1), 0, 2));
        $w->revokeBadge('fwu_1', 'fbg_1');
        $this->assertSame(['POST', '/api/field-workers/fwu_1/badges/fbg_1/revoke'], array_slice($this->seen(2), 0, 2));

        $bulk = $w->bulkIssueBadges(['fwu_1']);
        $this->assertSame(['POST', '/api/field-workers/badges'], array_slice($this->seen(3), 0, 2));
        $this->assertSame(['workerIds' => ['fwu_1']], json_decode($this->seen(3)[3], true));
        $this->assertSame('t1', $bulk?->getIssued()[0]->getToken());

        $code = $w->createSignInCode('fwu_1', 'depot');
        $this->assertSame(['POST', '/api/field-workers/fwu_1/sign-in-codes'], array_slice($this->seen(4), 0, 2));
        $this->assertSame(['appCode' => 'depot'], json_decode($this->seen(4)[3], true));
        $this->assertSame('ABC', $code?->getCode());

        $w->ensure((new EnsureFieldWorkerRequest())->setClientId('clt_1')->setWorkerCode('W1')->setName('Ann')->setIsTest(true)->setCanImpersonate(true));
        $this->assertEquals(['clientId' => 'clt_1', 'workerCode' => 'W1', 'name' => 'Ann', 'isTest' => true, 'canImpersonate' => true], json_decode($this->seen(5)[3], true));

        $w->list('clt_1', ['isTest' => true]);
        parse_str($this->seen(6)[2], $q);
        $this->assertSame(['clientId' => 'clt_1', 'isTest' => 'true'], $q);
    }

    public function test_link_unlink_and_linked_principal_filter(): void
    {
        $worker = ['id' => 'fwu_1', 'clientId' => 'clt_1', 'workerCode' => 'W1', 'name' => 'Ann'];
        $c = $this->client([
            $this->json($worker + ['linkedPrincipal' => ['id' => 'prn_1', 'name' => 'Pat', 'email' => 'pat@x.test']]),
            $this->json($worker),
            $this->json(['workers' => [], 'total' => 0, 'page' => 0, 'size' => 100]),
        ]);
        $w = $c->fieldWorkers();

        $linked = $w->link('fwu_1', 'prn_1');
        $this->assertSame(['PUT', '/api/field-workers/fwu_1/link'], array_slice($this->seen(0), 0, 2));
        $this->assertSame(['principalId' => 'prn_1'], json_decode($this->seen(0)[3], true));
        $this->assertSame('prn_1', $linked?->getLinkedPrincipal()?->getId());

        $w->unlink('fwu_1');
        $this->assertSame(['DELETE', '/api/field-workers/fwu_1/link'], array_slice($this->seen(1), 0, 2));

        $w->list('clt_1', ['linkedPrincipalId' => 'prn_1']);
        parse_str($this->seen(2)[2], $q);
        $this->assertSame(['clientId' => 'clt_1', 'linkedPrincipalId' => 'prn_1'], $q);
    }

    public function test_devices_policies_and_introspection_hit_the_right_endpoints(): void
    {
        $c = $this->client([
            $this->json(['code' => 'X', 'qrPayload' => 'fcfield:1:u:X'], 201),
            $this->json(['devices' => [], 'total' => 0, 'page' => 0, 'size' => 100]),
            $this->json(['id' => 'fwd_1']), $this->nothing(), $this->nothing(), $this->nothing(),
            $this->json(['clientId' => 'clt_1']), $this->json(['clientId' => 'clt_1']),
            $this->json(['active' => true, 'sub' => 'fwu_1']),
        ]);
        $d = $c->fieldDevices();

        $d->createEnrolmentCode((new CreateEnrolmentCodeRequest())->setClientId('clt_1')->setMode('PERSONAL')->setAppCodes(['a']));
        $this->assertSame(['POST', '/api/field-devices/enrolment-codes'], array_slice($this->seen(0), 0, 2));
        $d->list('clt_1', ['status' => 'ACTIVE']);
        $this->assertSame(['GET', '/api/field-devices'], array_slice($this->seen(1), 0, 2));
        $this->assertSame('clientId=clt_1&status=ACTIVE', $this->seen(1)[2]);
        $d->get('fwd_1');
        $this->assertSame(['GET', '/api/field-devices/fwd_1'], array_slice($this->seen(2), 0, 2));
        $d->suspend('fwd_1');
        $this->assertSame(['POST', '/api/field-devices/fwd_1/suspend'], array_slice($this->seen(3), 0, 2));
        $d->activate('fwd_1');
        $this->assertSame(['POST', '/api/field-devices/fwd_1/activate'], array_slice($this->seen(4), 0, 2));
        $d->revoke('fwd_1');
        $this->assertSame(['POST', '/api/field-devices/fwd_1/revoke'], array_slice($this->seen(5), 0, 2));

        $c->fieldPolicies()->get('clt_1');
        $this->assertSame(['GET', '/api/field-policies/clt_1'], array_slice($this->seen(6), 0, 2));
        $c->fieldPolicies()->put('clt_1', (new PutFieldPolicyRequest())->setPinMinLength(6));
        $this->assertSame(['PUT', '/api/field-policies/clt_1'], array_slice($this->seen(7), 0, 2));

        $r = $c->introspectFieldToken('the.token.here');
        $this->assertSame(['POST', '/api/field-tokens/introspect'], array_slice($this->seen(8), 0, 2));
        $this->assertSame(['token' => 'the.token.here'], json_decode($this->seen(8)[3], true));
        $this->assertTrue($r->getActive());
        $this->assertSame('fwu_1', $r->getSub());
    }
}
