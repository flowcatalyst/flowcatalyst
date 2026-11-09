<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Unit\Push;

use FlowCatalyst\Push\PushTicketException;
use FlowCatalyst\Push\TicketKey;
use FlowCatalyst\Push\TicketMinter;
use PHPUnit\Framework\TestCase;

/**
 * conformance/push/tickets.json of the framework (pinned copy). The keys in it
 * are test keys, published on purpose; EdDSA is deterministic, so the helper
 * must reproduce a valid ticket byte for byte, and its output must verify
 * against the vector's own expectations.
 */
final class TicketMinterTest extends TestCase
{
    private static object $v;

    public static function setUpBeforeClass(): void
    {
        self::$v = json_decode((string) file_get_contents(__DIR__ . '/../../Fixtures/push/tickets.json'), false, 512, JSON_THROW_ON_ERROR);
    }

    private static function cfg(string $name): string
    {
        $k = self::$v->keys->{$name};
        return "{$k->kid}:{$k->seed}";
    }

    /** @return array{header: array<string, mixed>, claims: array<string, mixed>, signingInput: string, sig: string}|null */
    private static function decode(string $token): ?array
    {
        $parts = explode('.', $token);
        if (count($parts) !== 3) {
            return null;
        }
        $h = TicketKey::base64UrlDecode($parts[0]);
        $c = TicketKey::base64UrlDecode($parts[1]);
        $s = TicketKey::base64UrlDecode($parts[2]);
        if ($h === null || $c === null || $s === null) {
            return null;
        }
        $header = json_decode($h, true);
        $claims = json_decode($c, true);
        if (!is_array($header) || !is_array($claims)) {
            return null;
        }
        return ['header' => $header, 'claims' => $claims, 'signingInput' => "{$parts[0]}.{$parts[1]}", 'sig' => $s];
    }

    /**
     * A ticket checker per push contract 6.4 for a host that knows one application and the
     * JWKS; returns the vector's "ticketReasons" word, or null when the ticket is valid.
     *
     * @param array{keys: array<mixed>} $jwks
     * @param array{client: ?string, proofKey: ?string} $ctx
     * @param array{value: ?string} $stream the stream's client, set by the first valid ticket
     */
    private static function reasonOf(string $token, array $jwks, int $now, array $ctx, array &$stream): ?string
    {
        $t = self::decode($token);
        if ($t === null) {
            return 'malformed';
        }
        if (($t['header']['alg'] ?? null) !== 'EdDSA') {
            return 'alg';
        }
        if (($t['claims']['iss'] ?? null) !== self::$v->application) {
            return 'iss';
        }
        $jwk = null;
        foreach ($jwks['keys'] as $k) {
            if (($k['kid'] ?? null) === ($t['header']['kid'] ?? null)) {
                $jwk = $k;
            }
        }
        if ($jwk === null) {
            return 'kid';
        }
        $pub = TicketKey::base64UrlDecode($jwk['x']);
        if (!sodium_crypto_sign_verify_detached($t['sig'], $t['signingInput'], (string) $pub)) {
            return 'signature';
        }
        if (($t['claims']['aud'] ?? null) !== 'fw-push') {
            return 'aud';
        }
        $iat = $t['claims']['iat'];
        $exp = $t['claims']['exp'];
        if ($exp <= $iat || $exp - $iat > 300) {
            return 'times';
        }
        if ($exp <= $now) {
            return 'expired';
        }
        foreach ($t['claims']['topics'] as $pattern) {
            if (explode(':', $pattern)[0] !== self::$v->application) {
                return 'topic-outside-iss';
            }
        }
        $jkt = $t['claims']['cnf']['jkt'] ?? null;
        if ($jkt !== null && $jkt !== $ctx['proofKey']) {
            return 'cnf';
        }
        $want = $ctx['client'] ?? $stream['value'];
        if ($want !== null && $t['claims']['client'] !== $want) {
            return 'client-mismatch';
        }
        if ($stream['value'] === null) {
            $stream['value'] = $t['claims']['client'];
        }
        return null;
    }

    /**
     * @param array{keys: array<mixed>} $jwks
     * @param string[]|null $tickets
     * @return list<?string>
     */
    private static function reasonsFor(object $case, array $jwks, ?array $tickets = null): array
    {
        $stream = ['value' => null];
        $out = [];
        foreach ($tickets ?? $case->tickets as $ticket) {
            $out[] = self::reasonOf($ticket, $jwks, self::$v->now, [
                'client' => $case->context->client ?? null,
                'proofKey' => $case->context->proofKey ?? null,
            ], $stream);
        }
        return $out;
    }

    private function vectorJwks(): array
    {
        return json_decode((string) json_encode(self::$v->jwks), true);
    }

    public function test_the_jwks_from_the_configured_keys_is_the_vectors_current_then_previous(): void
    {
        $m = TicketMinter::fromConfig(self::$v->application, self::cfg('current'), [self::cfg('previous')]);
        $this->assertEquals($this->vectorJwks(), $m->jwks());

        // a public JWK is enough for a previous key
        $m2 = new TicketMinter(self::$v->application, TicketKey::parse(self::cfg('current')), [$this->vectorJwks()['keys'][1]]);
        $this->assertEquals($this->vectorJwks(), $m2->jwks());
    }

    public function test_the_vectors_tickets_are_judged_as_the_vector_says(): void
    {
        $checked = 0;
        foreach (self::$v->cases as $case) {
            if (!isset($case->expect->ticketReasons)) {
                continue;
            }
            $this->assertSame($case->expect->ticketReasons, self::reasonsFor($case, $this->vectorJwks()), $case->name);
            $checked++;
        }
        $this->assertGreaterThanOrEqual(15, $checked);
    }

    public function test_the_helper_reproduces_the_vectors_tickets_byte_for_byte_and_refuses_what_it_must_not_mint(): void
    {
        $seeds = [self::$v->keys->current->seed, self::$v->keys->previous->seed, self::$v->keys->stranger->seed];
        $reproduced = 0;
        $refused = [];
        foreach (self::$v->cases as $case) {
            foreach ($case->tickets as $i => $token) {
                $t = self::decode($token);
                if ($t === null || ($t['header']['alg'] ?? null) !== 'EdDSA') {
                    continue;
                }
                $reason = $case->expect->ticketReasons[$i] ?? null;
                $c = $t['claims'];
                $minted = null;
                $wasRefused = false;
                foreach ($seeds as $seed) {
                    // the key bound to the ticket's kid, whichever seed signed it
                    $key = TicketKey::fromSeed((string) $t['header']['kid'], $seed);
                    try {
                        $out = (new TicketMinter($c['iss'], $key))->mint(
                            client: $c['client'],
                            topics: $c['topics'],
                            sub: $c['sub'] ?? null,
                            jkt: $c['cnf']['jkt'] ?? null,
                            ttlSeconds: $c['exp'] - $c['iat'],
                            streamExp: $c['stream-exp'] ?? null,
                            now: $c['iat'],
                        );
                    } catch (PushTicketException) {
                        $refused[] = $reason ?? '?';
                        $wasRefused = true;
                        break;
                    }
                    if ($out === $token) {
                        $minted = $out;
                        break;
                    }
                }
                if ($wasRefused) {
                    continue;
                }
                if ($reason === null) {
                    $this->assertSame($token, $minted, "{$case->name}: a valid ticket is reproduced");
                }
                if ($minted !== null) {
                    $reproduced++;
                } else {
                    $this->assertNotNull($reason, "{$case->name}: only an invalid ticket is out of the helper's reach");
                }
            }
        }
        $this->assertGreaterThanOrEqual(12, $reproduced);
        // exp more than 5 minutes after iat, and a topic outside iss, cannot be minted at all
        $this->assertContains('times', $refused);
        $this->assertContains('topic-outside-iss', $refused);
    }

    public function test_tickets_the_helper_mints_at_the_vectors_time_verify_with_the_vectors_expectations(): void
    {
        $now = self::$v->now;
        $app = self::$v->application;
        $m = TicketMinter::fromConfig($app, self::cfg('current'), [self::cfg('previous')]);
        $jwks = $m->jwks();
        $check = static function (string $ticket, array $jwks, ?string $client, ?string $proof = null) use ($now): array {
            $stream = ['value' => null];
            $reason = self::reasonOf($ticket, $jwks, $now, ['client' => $client, 'proofKey' => $proof], $stream);
            return [$reason, $stream['value']];
        };

        $t1 = $m->mint(client: 'acme', topics: ['fulfil:order:{id}'], sub: 'prn_1', now: $now - 10);
        $this->assertSame([null, 'acme'], $check($t1, $jwks, null), "the first valid ticket names the stream's client");
        $this->assertSame('client-mismatch', $check($t1, $jwks, 'other')[0]);

        $t2 = $m->mint(client: 'acme', topics: ['fulfil:order:{id}'], jkt: 'jkt-A', now: $now - 10);
        $this->assertNull($check($t2, $jwks, 'acme', 'jkt-A')[0]);
        $this->assertSame('cnf', $check($t2, $jwks, 'acme', 'jkt-B')[0]);

        $t3 = $m->mint(client: '_platform', topics: ['fulfil:order:{id}'], now: $now - 10);
        $this->assertSame([null, '_platform'], $check($t3, $jwks, null));

        $t4 = $m->mint(client: 'acme', topics: ['fulfil:order:{id}'], ttlSeconds: 60, now: $now - 100);
        $this->assertSame('expired', $check($t4, $jwks, 'acme')[0]);

        $old = TicketMinter::fromConfig($app, self::cfg('previous'));
        $t5 = $old->mint(client: 'acme', topics: ['fulfil:order:{id}'], now: $now);
        $this->assertNull($check($t5, $jwks, 'acme')[0], 'signed with the previous key');
        $this->assertSame(
            'kid',
            $check($t5, TicketMinter::fromConfig($app, self::cfg('current'))->jwks(), 'acme')[0],
            'the previous key dropped from the JWKS: unknown kid',
        );

        // header and claims order are the contract's
        $d1 = self::decode($t1);
        $this->assertSame(['alg', 'kid'], array_keys($d1['header']));
        $this->assertSame(['iss', 'aud', 'client', 'topics', 'sub', 'iat', 'exp'], array_keys($d1['claims']));
        $this->assertSame('fw-push', $d1['claims']['aud']);
        $d2 = self::decode($m->mint(client: 'acme', topics: ['fulfil:a:{x}'], sub: 'p', jkt: 'k', streamExp: new \DateTimeImmutable('@' . ($now + 60)), now: $now));
        $this->assertSame(['iss', 'aud', 'client', 'topics', 'sub', 'cnf', 'iat', 'exp', 'stream-exp'], array_keys($d2['claims']));
        $this->assertSame($now + 60, $d2['claims']['stream-exp']);
    }

    public function test_refuses_what_the_gateway_would_grant_nothing_for(): void
    {
        $m = TicketMinter::fromConfig('shop', 'k1:' . self::$v->keys->current->seed);
        $bad = [
            ['topics' => ['orders:order:{id}']],
            ['topics' => ['shop']],
            ['topics' => []],
            ['topics' => array_map(static fn($i) => "shop:t:{$i}", range(1, 101))],
            ['topics' => ['shop:a'], 'ttlSeconds' => 301],
            ['topics' => ['shop:a'], 'ttlSeconds' => 0],
            ['topics' => ['shop:a'], 'client' => 'a b'],
        ];
        foreach ($bad as $i => $args) {
            try {
                $m->mint(...['client' => 'acme', ...$args]);
                $this->fail("case {$i} should be refused");
            } catch (PushTicketException) {
                $this->addToAssertionCount(1);
            }
        }
        $this->assertNotSame('', $m->mint(client: 'acme', topics: array_map(static fn($i) => "shop:t:{$i}", range(1, 100))));
        $this->assertNotSame('', $m->mint(client: 'acme', topics: ['shop:a'], ttlSeconds: 300));
    }

    public function test_key_configuration_is_kid_colon_seed_with_a_32_byte_seed(): void
    {
        foreach (['noseed', ':abc', 'k:AAAA', str_repeat('k', 65) . ':' . self::$v->keys->current->seed] as $bad) {
            try {
                TicketKey::parse($bad);
                $this->fail("{$bad} should be refused");
            } catch (PushTicketException) {
                $this->addToAssertionCount(1);
            }
        }
    }

    public function test_the_jwks_carries_no_private_member_and_the_default_lifetime_is_60_seconds(): void
    {
        $m = TicketMinter::fromConfig('shop', 'k1:' . self::$v->keys->current->seed);
        $keys = $m->jwks()['keys'];
        $names = array_keys($keys[0]);
        sort($names);
        $this->assertSame(['alg', 'crv', 'kid', 'kty', 'use', 'x'], $names);
        $t = self::decode($m->mint(client: 'acme', topics: ['shop:a'], now: 1000));
        $this->assertSame(60, $t['claims']['exp'] - $t['claims']['iat']);
    }
}
