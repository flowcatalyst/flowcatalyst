<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Unit\Auth\Support;

use FlowCatalyst\Auth\Support\AccessTokenValidator;
use FlowCatalyst\Auth\Support\JwksCache;
use FlowCatalyst\Tests\Support\MockIssuer;
use GuzzleHttp\Client;
use Illuminate\Cache\ArrayStore;
use Illuminate\Cache\Repository;
use PHPUnit\Framework\TestCase;

/**
 * Covers P2: AccessTokenValidator's docblock claimed it checked `iss`, but
 * nothing ever compared it — any token signed by a JWKS key would pass
 * regardless of which platform issued it.
 */
final class AccessTokenValidatorTest extends TestCase
{
    private MockIssuer $issuer;

    protected function setUp(): void
    {
        $this->issuer = new MockIssuer();
    }

    protected function tearDown(): void
    {
        $this->issuer->stop();
    }

    private function validator(): AccessTokenValidator
    {
        $jwks = new JwksCache(new Client(), new Repository(new ArrayStore()));
        return new AccessTokenValidator($jwks, $this->issuer->baseUrl);
    }

    private function baseClaims(array $overrides = []): array
    {
        $now = time();
        return array_merge([
            'sub' => 'usr_1',
            'iss' => $this->issuer->issuer,
            'aud' => 'flowcatalyst',
            'exp' => $now + 300,
            'nbf' => $now - 5,
            'iat' => $now,
            'token_use' => 'api',
        ], $overrides);
    }

    public function test_foreign_issuer_is_rejected(): void
    {
        $token = $this->issuer->sign($this->baseClaims(['iss' => 'https://a-foreign-platform.test']));

        $this->assertNull($this->validator()->validate($token));
    }

    public function test_correct_issuer_is_accepted(): void
    {
        $token = $this->issuer->sign($this->baseClaims());

        $this->assertNotNull($this->validator()->validate($token));
    }

    public function test_token_use_is_an_allowlist(): void
    {
        foreach (['field', 'identity', 'portal', 'id', 'something-new', ''] as $use) {
            $token = $this->issuer->sign($this->baseClaims(['token_use' => $use]));
            $this->assertNull($this->validator()->validate($token), "token_use={$use} must be refused");
        }
        // Non-string markers are not "api" either.
        $this->assertNull($this->validator()->validate(
            $this->issuer->sign($this->baseClaims(['token_use' => ['api']]))
        ));

        $this->assertNotNull($this->validator()->validate($this->issuer->sign($this->baseClaims(['token_use' => 'api']))));
        $claims = $this->baseClaims();
        unset($claims['token_use']);
        $this->assertNotNull($this->validator()->validate($this->issuer->sign($claims)));
    }
}
