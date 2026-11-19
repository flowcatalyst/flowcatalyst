<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Unit\Auth\Support;

use FlowCatalyst\Auth\FlowCatalystTokenGuard;
use FlowCatalyst\Auth\Support\AccessTokenValidator;
use FlowCatalyst\Auth\Support\IdTokenValidator;
use FlowCatalyst\Auth\Support\JwksCache;
use FlowCatalyst\Auth\Support\JwtSignature;
use FlowCatalyst\Tests\Support\MockIssuer;
use FlowCatalyst\Tests\Support\Respellings;
use GuzzleHttp\Client;
use Illuminate\Cache\ArrayStore;
use Illuminate\Cache\Repository;
use Illuminate\Http\Request;
use PHPUnit\Framework\TestCase;

/**
 * A platform token verifies only in the spelling it was issued in
 * (docs/wire-contract.md, "Token spelling"): every re-spelling and every
 * change in content is refused by JwtSignature and so by the access-token and
 * ID-token validators and the guard built on them, and the issued token still
 * verifies afterwards. The platform's ID tokens are the platform's own tokens:
 * strict like the rest.
 */
final class CanonicalSpellingTest extends TestCase
{
    private const CLIENT_ID = 'oac_canonical';

    private MockIssuer $issuer;

    protected function setUp(): void
    {
        $this->issuer = new MockIssuer();
    }

    protected function tearDown(): void
    {
        $this->issuer->stop();
    }

    private function jwks(): JwksCache
    {
        return new JwksCache(new Client(), new Repository(new ArrayStore()));
    }

    /** @return array<string, mixed> */
    private function claims(string $aud, string $use = 'api'): array
    {
        $now = time();
        return [
            'sub' => 'prn_canonical', 'iss' => $this->issuer->issuer, 'aud' => $aud,
            'exp' => $now + 300, 'nbf' => $now - 5, 'iat' => $now, 'token_use' => $use,
        ];
    }

    /**
     * @param callable(string): bool $accepts
     */
    private function assertOnlyTheIssuedSpelling(string $token, callable $accepts, bool $surroundingWhitespace = true): void
    {
        $this->assertTrue($accepts($token), 'the issued token must verify');
        $cases = Respellings::of($token, $surroundingWhitespace);
        // A 2048-bit RS256 signature is 342 characters: 4 spare bits.
        $this->assertCount(15 + 6 + ($surroundingWhitespace ? 3 : 0), $cases);
        foreach ($cases as $name => $bad) {
            $this->assertNotSame($token, $bad);
            $this->assertFalse($accepts($bad), "re-spelled ({$name}) must be refused");
        }
        foreach (Respellings::tampered($token, 'prn_canonical', 'prn_somebody') as $name => $bad) {
            $this->assertNotSame($token, $bad);
            $this->assertFalse($accepts($bad), "tampered ({$name}) must be refused");
        }
        $this->assertTrue($accepts($token), 'the issued token must still verify');
    }

    public function test_access_tokens_verify_only_as_issued(): void
    {
        $v = new AccessTokenValidator($this->jwks(), $this->issuer->baseUrl);
        $this->assertOnlyTheIssuedSpelling(
            $this->issuer->sign($this->claims('flowcatalyst')),
            static fn (string $t): bool => $v->validate($t) !== null,
        );
    }

    public function test_id_tokens_verify_only_as_issued(): void
    {
        $v = new IdTokenValidator($this->jwks(), $this->issuer->baseUrl, self::CLIENT_ID);
        $claims = $this->claims(self::CLIENT_ID);
        unset($claims['token_use']);
        $this->assertOnlyTheIssuedSpelling(
            $this->issuer->sign($claims),
            static fn (string $t): bool => $v->validate($t) !== null,
        );
    }

    /**
     * The guard reads `Authorization: Bearer <token>`; whitespace around the
     * header value and between the scheme and the token is the header's
     * framing (HTTP strips it), and the token it passes on must be canonical.
     */
    public function test_the_bearer_guard_refuses_a_re_spelled_token(): void
    {
        $guard = new FlowCatalystTokenGuard(new AccessTokenValidator($this->jwks(), $this->issuer->baseUrl));
        $request = static function (string $token): Request {
            $r = Request::create('/api/orders');
            $r->headers->set('Authorization', 'Bearer ' . $token);
            return $r;
        };
        $this->assertOnlyTheIssuedSpelling(
            $this->issuer->sign($this->claims('flowcatalyst')),
            static fn (string $t): bool => $guard->resolve($request($t)) !== null,
            surroundingWhitespace: false,
        );
    }

    public function test_canonical_is_exactly_the_encoders_output(): void
    {
        for ($n = 0; $n < 10; $n++) {
            $raw = '';
            for ($i = 0; $i < $n; $i++) {
                $raw .= chr(0xff - $i);
            }
            $s = Respellings::enc($raw);
            $this->assertTrue(JwtSignature::isCanonical($s), "length {$n}: {$s}");
            $this->assertSame($raw, JwtSignature::decodeCanonical($s));
        }
        $sig = Respellings::enc("\xfb\xff\xbf\xfe"); // "-_-_-g": 4 spare bits
        $bad = Respellings::nonCanonicalSpellings($sig);
        $this->assertCount(15, $bad);
        array_push($bad, $sig . '==', $sig . '=', '+/+/+g', "-_\n-_-g", "-_\r-_-g", $sig . "\r\n", ' ' . $sig, $sig . ' ', "-_\t-_-g", 'A');
        foreach ($bad as $s) {
            $this->assertFalse(JwtSignature::isCanonical($s), json_encode($s));
            $this->assertNull(JwtSignature::decodeCanonical($s));
        }
        $this->assertTrue(JwtSignature::isCanonicalCompact('eyJhbGciOiJIUzI1NiJ9.e30.AAAAAA'));
        foreach (['', 'a.b', 'a.b.c.d', 'eyJhbGciOiJIUzI1NiJ9.e30.AAAAAB', 'eyJhbGciOiJIUzI1NiJ9.e30=.AAAAAA', "eyJhbGciOiJIUzI1NiJ9.e30.AAAAAA\n"] as $s) {
            $this->assertFalse(JwtSignature::isCanonicalCompact($s), json_encode($s));
        }
    }
}
