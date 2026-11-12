<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Unit\Auth\Support;

use FlowCatalyst\Auth\Support\JwksCache;
use FlowCatalyst\Auth\Support\JwtSignature;
use GuzzleHttp\Client;
use GuzzleHttp\Handler\MockHandler;
use GuzzleHttp\HandlerStack;
use GuzzleHttp\Promise\Create;
use GuzzleHttp\Psr7\Response;
use Illuminate\Cache\ArrayStore;
use Illuminate\Cache\Repository;
use PHPUnit\Framework\TestCase;
use Psr\Http\Message\RequestInterface;

/**
 * L14: a token carrying an unknown `kid` must not trigger a discovery+JWKS
 * refetch per request. One forced refetch per issuer per window; a failed
 * refetch keeps the cached set.
 */
final class JwtSignatureRefetchTest extends TestCase
{
    private const BASE = 'https://fc.test';

    /** @var array<string, array{0: \OpenSSLAsymmetricKey, 1: array<string, string>}> */
    private array $keys = [];
    /** @var list<string> kids currently published */
    private array $published = [];
    private int $fetches = 0;
    private bool $failing = false;
    private int $clock = 1_000_000;

    private function addKey(string $kid): void
    {
        $res = openssl_pkey_new(['private_key_bits' => 2048, 'private_key_type' => OPENSSL_KEYTYPE_RSA]);
        $d = openssl_pkey_get_details($res);
        $this->keys[$kid] = [$res, [
            'kty' => 'RSA', 'kid' => $kid, 'alg' => 'RS256', 'use' => 'sig',
            'n' => self::b64u($d['rsa']['n']), 'e' => self::b64u($d['rsa']['e']),
        ]];
    }

    private static function b64u(string $s): string
    {
        return rtrim(strtr(base64_encode($s), '+/', '-_'), '=');
    }

    private function sign(string $kid): string
    {
        $h = self::b64u(json_encode(['alg' => 'RS256', 'kid' => $kid]));
        $p = self::b64u(json_encode(['sub' => 'u']));
        openssl_sign("$h.$p", $sig, $this->keys[$kid][0], OPENSSL_ALGO_SHA256);
        return "$h.$p." . self::b64u($sig);
    }

    private function cache(): JwksCache
    {
        $handler = function (RequestInterface $req) {
            $url = (string) $req->getUri();
            if (str_ends_with($url, '/.well-known/openid-configuration')) {
                $this->fetches++;
                if ($this->failing) {
                    return Create::promiseFor(new Response(500, [], 'boom'));
                }
                return Create::promiseFor(new Response(200, [], json_encode([
                    'issuer' => self::BASE, 'jwks_uri' => self::BASE . '/jwks',
                ])));
            }
            $set = array_map(fn ($k) => $this->keys[$k][1], $this->published);
            return Create::promiseFor(new Response(200, [], json_encode(['keys' => array_values($set)])));
        };
        return new JwksCache(
            new Client(['handler' => HandlerStack::create($handler)]),
            new Repository(new ArrayStore()),
            300,
            60,
            fn () => $this->clock,
        );
    }

    public function test_unknown_kids_inside_window_cause_exactly_one_refetch(): void
    {
        $this->addKey('k1');
        $this->published = ['k1'];
        $jwks = $this->cache();
        $this->assertNotNull(JwtSignature::verify($this->sign('k1'), $jwks, self::BASE));
        $this->assertSame(1, $this->fetches);

        for ($i = 0; $i < 20; $i++) {
            $this->addKey("rogue$i");
            $this->assertNull(JwtSignature::verify($this->sign("rogue$i"), $jwks, self::BASE));
        }
        // 1 initial fill + exactly 1 forced refetch for 20 unknown kids.
        $this->assertSame(2, $this->fetches);

        // Known kid unaffected, no network.
        $this->assertNotNull(JwtSignature::verify($this->sign('k1'), $jwks, self::BASE));
        $this->assertSame(2, $this->fetches);
    }

    public function test_another_refetch_is_allowed_after_the_window_and_picks_up_new_key(): void
    {
        $this->addKey('k1');
        $this->published = ['k1'];
        $jwks = $this->cache();
        JwtSignature::verify($this->sign('k1'), $jwks, self::BASE);

        $this->addKey('k2');
        $this->assertNull(JwtSignature::verify($this->sign('k2'), $jwks, self::BASE)); // forced, not yet published
        $this->published = ['k1', 'k2'];
        $this->clock += 30;
        // Still inside the window: refused with no network call.
        $this->assertNull(JwtSignature::verify($this->sign('k2'), $jwks, self::BASE));
        $before = $this->fetches;
        $this->clock += 31;
        $this->assertNotNull(JwtSignature::verify($this->sign('k2'), $jwks, self::BASE));
        $this->assertSame($before + 1, $this->fetches);
    }

    public function test_failed_refetch_keeps_previously_cached_keys(): void
    {
        $this->addKey('k1');
        $this->published = ['k1'];
        $jwks = $this->cache();
        $this->assertNotNull(JwtSignature::verify($this->sign('k1'), $jwks, self::BASE));

        $this->failing = true;
        $this->addKey('other');
        $this->assertNull(JwtSignature::verify($this->sign('other'), $jwks, self::BASE));
        $this->assertSame(2, $this->fetches, 'the forced refetch was attempted');

        $this->assertNotNull(
            JwtSignature::verify($this->sign('k1'), $jwks, self::BASE),
            'cached key set must survive a failed refetch',
        );
    }
}
