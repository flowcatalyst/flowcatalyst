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

    private function validator(bool $acceptIdTokenShapedTokens = false): AccessTokenValidator
    {
        $jwks = new JwksCache(new Client(), new Repository(new ArrayStore()));
        return new AccessTokenValidator(
            $jwks,
            $this->issuer->baseUrl,
            acceptIdTokenShapedTokens: $acceptIdTokenShapedTokens,
        );
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
            'token_use' => 'identity',
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
        foreach (['field', 'portal', 'id', 'something-new', ''] as $use) {
            $token = $this->issuer->sign($this->baseClaims(['token_use' => $use]));
            $this->assertNull($this->validator()->validate($token), "token_use={$use} must be refused");
        }
        // Non-string markers are not "api" either.
        $this->assertNull($this->validator()->validate(
            $this->issuer->sign($this->baseClaims(['token_use' => ['api']]))
        ));

        $this->assertNotNull($this->validator()->validate($this->issuer->sign($this->baseClaims(['token_use' => 'api']))));
        $this->assertNotNull($this->validator()->validate($this->issuer->sign($this->baseClaims(['token_use' => 'identity']))));
        $claims = $this->baseClaims();
        unset($claims['token_use']);
        $this->assertNotNull($this->validator()->validate($this->issuer->sign($claims)));
    }

    /** Claims of the platform's OIDC ID token: aud = OAuth client id, no token_use. */
    private function idTokenClaims(array $overrides = []): array
    {
        $claims = $this->baseClaims(array_merge([
            'aud' => 'oauth_client_1',
            'nonce' => 'n-0S6_WzA2Mj',
            'roles' => ['admin'],
            'clients' => ['*'],
            'tier' => 'ANCHOR',
        ], $overrides));
        unset($claims['token_use']);

        return $claims;
    }

    public function test_id_token_shaped_token_is_rejected_by_default(): void
    {
        $token = $this->issuer->sign($this->idTokenClaims());

        $this->assertNull($this->validator()->validate($token));
    }

    public function test_id_token_shaped_token_is_accepted_when_the_escape_hatch_is_on(): void
    {
        $token = $this->issuer->sign($this->idTokenClaims());

        $this->assertNotNull($this->validator(acceptIdTokenShapedTokens: true)->validate($token));
    }

    public function test_each_id_token_sign_is_enough(): void
    {
        $signs = [
            'nonce' => ['nonce' => 'abc'],
            'auth_time' => ['nonce' => null, 'auth_time' => time() - 10],
            'at_hash' => ['nonce' => null, 'at_hash' => 'HK6E_P6Dh8Y93mRNtsDB1Q'],
            'azp == aud' => ['nonce' => null, 'azp' => 'oauth_client_1'],
            'azp == [aud]' => ['nonce' => null, 'azp' => 'oauth_client_1', 'aud' => ['oauth_client_1']],
        ];
        foreach ($signs as $name => $overrides) {
            $claims = $this->idTokenClaims($overrides);
            $claims = array_filter($claims, static fn ($v) => $v !== null);
            $token = $this->issuer->sign($claims);

            $this->assertNull($this->validator()->validate($token), "{$name} must be refused");
            $this->assertNotNull($this->validator(true)->validate($token), "{$name} accepted with the escape hatch");
        }
    }

    public function test_api_token_is_still_accepted_even_with_id_token_claims(): void
    {
        $token = $this->issuer->sign($this->baseClaims(['token_use' => 'api', 'aud' => 'oauth_client_1', 'azp' => 'oauth_client_1']));

        $this->assertNotNull($this->validator()->validate($token));
    }

    public function test_identity_token_keeps_todays_behaviour(): void
    {
        $token = $this->issuer->sign($this->baseClaims(['token_use' => 'identity', 'nonce' => 'abc', 'azp' => 'flowcatalyst']));

        $this->assertNotNull($this->validator()->validate($token));
    }

    public function test_marker_less_token_without_id_token_signs_keeps_todays_behaviour(): void
    {
        $claims = $this->baseClaims(['azp' => 'some-other-client']); // azp != aud
        unset($claims['token_use']);
        $this->assertNotNull($this->validator()->validate($this->issuer->sign($claims)));

        // azp equal to one of several audiences is not "equal to its single aud".
        $claims = $this->baseClaims(['azp' => 'flowcatalyst', 'aud' => ['flowcatalyst', 'other']]);
        unset($claims['token_use']);
        $this->assertNotNull($this->validator()->validate($this->issuer->sign($claims)));

        $claims = $this->baseClaims();
        unset($claims['token_use']);
        $this->assertNotNull($this->validator()->validate($this->issuer->sign($claims)));
    }
}
