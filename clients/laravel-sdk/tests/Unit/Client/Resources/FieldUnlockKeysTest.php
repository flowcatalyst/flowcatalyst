<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Unit\Client\Resources;

use FlowCatalyst\Client\Auth\TokenProviderInterface;
use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\Generated\Client as GeneratedClient;
use FlowCatalyst\Client\Resources\FieldPolicies;
use FlowCatalyst\Generated\Model\PutFieldPolicyRequest;
use GuzzleHttp\Client;
use GuzzleHttp\Handler\MockHandler;
use GuzzleHttp\HandlerStack;
use GuzzleHttp\Middleware;
use GuzzleHttp\Psr7\Response;
use PHPUnit\Framework\TestCase;

/**
 * Unlock keys (a biometric sign-in on a personal device): the two admin calls, the device's key and
 * the policy's settings, and that a get followed by a put changes nothing.
 */
final class FieldUnlockKeysTest extends TestCase
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

    private function key(array $over = []): array
    {
        return $over + [
            'id' => 'fuk_1', 'workerId' => 'fwu_1', 'deviceId' => 'fdv_1', 'status' => 'ACTIVE',
            'createdAt' => '2026-01-01T00:00:00Z', 'pinVerifiedAt' => '2026-01-01T00:00:00Z', 'liveSessions' => 2,
            'attestation' => ['format' => 'ANDROID_KEY', 'verdict' => 'TRUSTED', 'securityLevel' => 'STRONGBOX',
                'challengeMatches' => true, 'authPerUse' => true, 'biometricOnly' => true, 'signP256' => true,
                'meetsRequirement' => true],
        ];
    }

    private function policy(): array
    {
        return [
            'clientId' => 'clt_1', 'pinMinLength' => 8, 'softLockThreshold' => 4, 'softLockSecs' => 600,
            'hardLockThreshold' => 12, 'sharedIdleSecs' => 300, 'personalIdleSecs' => 3600, 'shiftMaxSecs' => 43200,
            'requireAttestation' => true, 'pinReauthDays' => 0, 'requireEnrolledDevice' => true,
            'unenrolledIdleSecs' => 120, 'badgeOnlyLogin' => true, 'impersonationMaxSecs' => 1800,
            'workerCodeDigits' => 5, 'photosEnabled' => false, 'workerMaySetOwnPhoto' => true,
            'unlockKeysEnabled' => true, 'unlockKeyAttestation' => 'REQUIRED', 'isDefault' => false,
        ];
    }

    public function test_list_and_revoke_unlock_keys(): void
    {
        $revoked = $this->key(['status' => 'REVOKED', 'revokedReason' => 'ADMIN', 'revokedAt' => '2026-01-03T00:00:00Z', 'liveSessions' => 0]);
        $c = $this->client([$this->json(['unlockKeys' => [$this->key()]]), $this->json($revoked)]);

        $list = $c->fieldWorkers()->listUnlockKeys('fwu_1');
        $this->assertSame(['GET', '/api/field-workers/fwu_1/unlock-keys'], array_slice($this->seen(0), 0, 2));
        $key = $list?->getUnlockKeys()[0];
        $this->assertSame('fuk_1', $key?->getId());
        $this->assertSame(2, $key?->getLiveSessions());
        $this->assertSame('TRUSTED', $key?->getAttestation()?->getVerdict());
        $this->assertTrue($key?->getAttestation()?->getMeetsRequirement());

        $r = $c->fieldWorkers()->revokeUnlockKey('fwu_1', 'fuk_1');
        $this->assertSame(['POST', '/api/field-workers/fwu_1/unlock-keys/fuk_1/revoke'], array_slice($this->seen(1), 0, 2));
        $this->assertSame('REVOKED', $r?->getStatus());
        $this->assertSame('ADMIN', $r?->getRevokedReason());
        $this->assertSame(0, $r?->getLiveSessions());
    }

    public function test_a_device_carries_its_unlock_key_or_null(): void
    {
        $device = ['id' => 'fdv_1', 'clientId' => 'clt_1', 'mode' => 'PERSONAL', 'jkt' => 'j', 'attestationLevel' => 'TEE',
            'status' => 'ACTIVE', 'apps' => [], 'activeSessions' => 0, 'enrolledAt' => '2026-01-01T00:00:00Z'];
        $c = $this->client([$this->json($device + ['unlockKey' => $this->key()]), $this->json($device + ['unlockKey' => null])]);

        $this->assertSame('fuk_1', $c->fieldDevices()->get('fdv_1')?->getUnlockKey()?->getId());
        $this->assertNull($c->fieldDevices()->get('fdv_1')?->getUnlockKey());
    }

    public function test_to_request_carries_every_setting(): void
    {
        $c = $this->client([$this->json($this->policy()), $this->json($this->policy())]);

        $policy = $c->fieldPolicies()->get('clt_1');
        $this->assertSame('REQUIRED', $policy?->getUnlockKeyAttestation());
        $c->fieldPolicies()->put('clt_1', FieldPolicies::toRequest($policy));
        $expected = $this->policy();
        unset($expected['clientId'], $expected['isDefault']);
        $this->assertEquals($expected, json_decode($this->seen(1)[3], true));
    }

    public function test_update_reads_changes_one_setting_and_keeps_unlock_keys_on(): void
    {
        $c = $this->client([$this->json($this->policy()), $this->json($this->policy())]);

        $c->fieldPolicies()->update('clt_1', static fn (PutFieldPolicyRequest $r) => $r->setPinMinLength(10));
        $this->assertSame('GET', $this->seen(0)[0]);
        $this->assertSame('PUT', $this->seen(1)[0]);
        $sent = json_decode($this->seen(1)[3], true);
        $this->assertSame(10, $sent['pinMinLength']);
        $this->assertTrue($sent['unlockKeysEnabled']);
        $this->assertSame('REQUIRED', $sent['unlockKeyAttestation']);
        $this->assertTrue($sent['workerMaySetOwnPhoto']);
        $this->assertSame(0, $sent['pinReauthDays']);
    }

    public function test_a_bare_put_without_the_unlock_settings_does_not_send_them(): void
    {
        $c = $this->client([$this->json($this->policy())]);

        $c->fieldPolicies()->put('clt_1', (new PutFieldPolicyRequest())->setRequireAttestation(true));
        $this->assertEquals(['requireAttestation' => true], json_decode($this->seen(0)[3], true));
    }
}
