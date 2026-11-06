<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Unit\Auth\Field;

use FlowCatalyst\Auth\Field\FieldTokenException;
use FlowCatalyst\Auth\Field\FieldTokenVerifier;
use FlowCatalyst\Auth\Support\JwksCache;
use GuzzleHttp\Client;
use GuzzleHttp\Handler\MockHandler;
use GuzzleHttp\HandlerStack;
use GuzzleHttp\Psr7\Response;
use Illuminate\Cache\ArrayStore;
use Illuminate\Cache\Repository;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

/**
 * Runs every shared conformance vector (api/field-fixtures) through the
 * verifier, plus alg-pinning and scheme checks built on the same vectors.
 * The fixtures sit outside this package; a standalone (mirrored) checkout
 * skips with a message instead of passing silently.
 */
final class FieldTokenVerifierTest extends TestCase
{
    public const FIXTURE_DIR = __DIR__ . '/../../../../../../api/field-fixtures';

    /** @return array<string, array{0: array<string, mixed>}> */
    public static function cases(): array
    {
        if (!is_file(self::FIXTURE_DIR . '/cases.json')) {
            return ['absent' => [['name' => '__absent__']]];
        }
        $out = [];
        foreach (self::load()['cases'] as $c) {
            $out[$c['name']] = [$c];
        }
        return $out;
    }

    /** @return array{issuer: string, appCode: string, cases: list<array<string, mixed>>} */
    private static function load(): array
    {
        /** @var array{issuer: string, appCode: string, cases: list<array<string, mixed>>} */
        return json_decode((string) file_get_contents(self::FIXTURE_DIR . '/cases.json'), true, 512, JSON_THROW_ON_ERROR);
    }

    private function verifierAt(string $now, ?string $appCode = null, int $proofMaxAge = 300): FieldTokenVerifier
    {
        $f = self::load();
        $mock = new MockHandler([
            new Response(200, [], json_encode(['issuer' => $f['issuer'], 'jwks_uri' => $f['issuer'] . '/jwks'])),
            new Response(200, [], (string) file_get_contents(self::FIXTURE_DIR . '/jwks.json')),
        ]);
        $cache = new JwksCache(new Client(['handler' => HandlerStack::create($mock)]), new Repository(new ArrayStore()));
        $ts = strtotime($now);
        return new FieldTokenVerifier($cache, $f['issuer'], $appCode ?? $f['appCode'], proofMaxAgeSeconds: $proofMaxAge, now: static fn (): int => $ts);
    }

    /** @param array<string, mixed> $case */
    #[DataProvider('cases')]
    public function test_fixture_case(array $case): void
    {
        if ($case['name'] === '__absent__') {
            $this->markTestSkipped('field fixtures not present at ' . self::FIXTURE_DIR . ' (standalone SDK checkout)');
        }
        $v = $this->verifierAt($case['now']);
        try {
            $p = $v->verify('DPoP ' . $case['accessToken'], $case['dpopProof'], $case['method'], $case['url']);
        } catch (FieldTokenException $e) {
            $this->assertSame($case['expect'], $e->reason, $case['name']);
            return;
        }
        $this->assertSame('valid', $case['expect'], $case['name'] . ' was accepted');
        $this->assertSame('fwu_0FIXTUREWORKER', $p->workerId);
        $this->assertSame('fixture-client', $p->clientIdentifier);
        $this->assertSame('W001', $p->workerCode);
        $this->assertSame('fixture-app', $p->appCode);
        $this->assertNotSame('', $p->jkt);
        $want = $case['principal'] ?? null;
        if ($want === null) {
            $this->assertSame(['pin'], $p->amr);
            $this->assertNotSame('', $p->deviceId);
            $this->assertNull($p->actor);
            $this->assertFalse($p->test);
            return;
        }
        $this->assertSame($want['deviceId'], $p->deviceId, $case['name']);
        $this->assertSame($want['deviceMode'], $p->deviceMode, $case['name']);
        $this->assertSame($want['amr'], $p->amr, $case['name']);
        $this->assertSame($want['test'], $p->test, $case['name']);
        if (!isset($want['actor'])) {
            $this->assertNull($p->actor, $case['name']);
        } else {
            $this->assertNotNull($p->actor, $case['name']);
            $this->assertSame($want['actor']['id'], $p->actor->id);
            $this->assertSame($want['actor']['type'], $p->actor->type);
            $this->assertSame($want['actor']['name'], $p->actor->name);
        }
    }

    public function test_sign_in_options_cases_are_in_the_fixture_set(): void
    {
        $this->valid();
        $names = array_column(array_filter(self::load()['cases'], static fn ($c) => isset($c['principal'])), 'name');
        foreach (['valid_unenrolled', 'valid_impersonated_supervisor', 'valid_impersonated_principal', 'valid_test'] as $n) {
            $this->assertContains($n, $names);
        }
    }

    /** @return array<string, mixed> */
    private function valid(): array
    {
        if (!is_file(self::FIXTURE_DIR . '/cases.json')) {
            $this->markTestSkipped('field fixtures not present (standalone SDK checkout)');
        }
        foreach (self::load()['cases'] as $c) {
            if ($c['name'] === 'valid') {
                return $c;
            }
        }
        $this->fail('no valid case');
    }

    private function refusal(callable $fn): string
    {
        try {
            $fn();
        } catch (FieldTokenException $e) {
            return $e->reason;
        }
        $this->fail('credential was accepted');
    }

    private static function b64u(array $a): string
    {
        return rtrim(strtr(base64_encode((string) json_encode($a)), '+/', '-_'), '=');
    }

    public function test_bearer_scheme_is_refused(): void
    {
        $c = $this->valid();
        $r = $this->refusal(fn () => $this->verifierAt($c['now'])->verify('Bearer ' . $c['accessToken'], $c['dpopProof'], $c['method'], $c['url']));
        $this->assertSame('malformed', $r);
    }

    public function test_scheme_is_case_insensitive(): void
    {
        $c = $this->valid();
        $p = $this->verifierAt($c['now'])->verify('dpop ' . $c['accessToken'], $c['dpopProof'], $c['method'], $c['url']);
        $this->assertSame('W001', $p->workerCode);
    }

    public function test_wrong_app_code_is_refused(): void
    {
        $c = $this->valid();
        $r = $this->refusal(fn () => $this->verifierAt($c['now'], 'another-app')->verify('DPoP ' . $c['accessToken'], $c['dpopProof'], $c['method'], $c['url']));
        $this->assertSame('wrong_aud', $r);
    }

    public function test_a_different_origin_does_not_match(): void
    {
        $c = $this->valid();
        $r = $this->refusal(fn () => $this->verifierAt($c['now'])->verify('DPoP ' . $c['accessToken'], $c['dpopProof'], $c['method'], 'https://evil.example/api/orders'));
        $this->assertSame('htu_mismatch', $r);
    }

    /** @return array<string, array{0: array<string, string>, 1: string}> */
    public static function foreignAlgs(): array
    {
        $payload = self::b64u(['iss' => 'https://fc.example.test', 'aud' => 'fixture-app', 'token_use' => 'field', 'exp' => 4102444800]);
        return [
            'HS256' => [['alg' => 'HS256', 'typ' => 'fc-field+jwt', 'kid' => 'fixture-key'], $payload . '.' . str_repeat('A', 43)],
            'none' => [['alg' => 'none', 'typ' => 'fc-field+jwt'], $payload . '.'],
            'ES256' => [['alg' => 'ES256', 'typ' => 'fc-field+jwt', 'kid' => 'fixture-key'], $payload . '.' . str_repeat('A', 86)],
            'RS512' => [['alg' => 'RS512', 'typ' => 'fc-field+jwt', 'kid' => 'fixture-key'], $payload . '.' . str_repeat('A', 342)],
        ];
    }

    /** @param array<string, string> $header */
    #[DataProvider('foreignAlgs')]
    public function test_token_alg_other_than_rs256_is_refused(array $header, string $rest): void
    {
        $c = $this->valid();
        $token = self::b64u($header) . '.' . $rest;
        $r = $this->refusal(fn () => $this->verifierAt($c['now'])->verify('DPoP ' . $token, $c['dpopProof'], $c['method'], $c['url']));
        $this->assertSame('invalid_token', $r);
    }

    public function test_proof_alg_other_than_es256_is_refused(): void
    {
        $c = $this->valid();
        [$h, $pl, $sig] = explode('.', $c['dpopProof']);
        $hdr = json_decode((string) base64_decode(strtr($h, '-_', '+/')), true);
        foreach (['HS256', 'none', 'RS256'] as $alg) {
            $hdr['alg'] = $alg;
            $proof = self::b64u($hdr) . ".{$pl}.{$sig}";
            $r = $this->refusal(fn () => $this->verifierAt($c['now'])->verify('DPoP ' . $c['accessToken'], $proof, $c['method'], $c['url']));
            $this->assertSame('invalid_proof', $r, $alg);
        }
    }

    public function test_proof_with_private_key_material_is_refused(): void
    {
        $c = $this->valid();
        [$h, $pl, $sig] = explode('.', $c['dpopProof']);
        $hdr = json_decode((string) base64_decode(strtr($h, '-_', '+/')), true);
        $hdr['jwk']['d'] = 'AAAA';
        $r = $this->refusal(fn () => $this->verifierAt($c['now'])->verify('DPoP ' . $c['accessToken'], self::b64u($hdr) . ".{$pl}.{$sig}", $c['method'], $c['url']));
        $this->assertSame('invalid_proof', $r);
    }

    public function test_widened_proof_window_accepts_a_stale_proof(): void
    {
        $this->valid();
        $stale = null;
        foreach (self::load()['cases'] as $x) {
            if ($x['name'] === 'stale_proof') {
                $stale = $x;
            }
        }
        $this->assertNotNull($stale);
        $v = $this->verifierAt($stale['now'], null, 86400 * 365);
        $p = $v->verify('DPoP ' . $stale['accessToken'], $stale['dpopProof'], $stale['method'], $stale['url']);
        $this->assertSame('W001', $p->workerCode);
    }
}
