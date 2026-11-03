<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Feature\Auth;

use FlowCatalyst\Auth\Field\FieldPrincipal;
use FlowCatalyst\Auth\Field\FieldTokenVerifier;
use FlowCatalyst\Auth\Support\JwksCache;
use FlowCatalyst\FlowCatalystServiceProvider;
use GuzzleHttp\Client;
use GuzzleHttp\Exception\ConnectException;
use GuzzleHttp\Handler\MockHandler;
use GuzzleHttp\HandlerStack;
use GuzzleHttp\Psr7\Request as PsrRequest;
use GuzzleHttp\Psr7\Response;
use Illuminate\Cache\ArrayStore;
use Illuminate\Cache\Repository;
use Illuminate\Support\Facades\Auth;
use Orchestra\Testbench\TestCase;

/**
 * The `field` guard and `fc.field` middleware, driven by the shared fixtures:
 * the request carries Host `localhost` while the proofs were signed for
 * https://app.example.test, so a pass proves the htu comes from config.
 */
final class FieldGuardTest extends TestCase
{
    private const DIR = __DIR__ . '/../../../../../api/field-fixtures';

    /** @var array<string, mixed> */
    private array $fixtures = [];

    protected function getPackageProviders($app): array
    {
        return [FlowCatalystServiceProvider::class];
    }

    protected function getEnvironmentSetUp($app): void
    {
        $app['config']->set('app.key', 'base64:' . base64_encode(str_repeat('a', 32)));
        $app['config']->set('flowcatalyst.base_url', 'https://fc.example.test');
        $app['config']->set('flowcatalyst.field.app_code', 'fixture-app');
        $app['config']->set('flowcatalyst.field.app_base_url', 'https://app.example.test');
    }

    protected function defineRoutes($router): void
    {
        $router->post('/api/orders', function () {
            /** @var FieldPrincipal $p */
            $p = request()->attributes->get('fc.field_principal');
            return [
                'worker' => $p->workerCode,
                'viaGuard' => Auth::guard('field')->user()?->getAuthIdentifier(),
                'viaRequest' => request()->user()?->getAuthIdentifier(),
            ];
        })->middleware('fc.field');
    }

    protected function setUp(): void
    {
        parent::setUp();
        if (!is_file(self::DIR . '/cases.json')) {
            $this->markTestSkipped('field fixtures not present at ' . self::DIR . ' (standalone SDK checkout)');
        }
        $this->fixtures = json_decode((string) file_get_contents(self::DIR . '/cases.json'), true, 512, JSON_THROW_ON_ERROR);
    }

    /** Swap the verifier for one that reads fixture JWKS (or fails) with a fixed clock. */
    private function useVerifier(string $now, bool $keysDown = false): void
    {
        $f = $this->fixtures;
        $mock = $keysDown
            ? new MockHandler([new ConnectException('down', new PsrRequest('GET', 'https://fc.example.test'))])
            : new MockHandler([
                new Response(200, [], json_encode(['issuer' => $f['issuer'], 'jwks_uri' => $f['issuer'] . '/jwks'])),
                new Response(200, [], (string) file_get_contents(self::DIR . '/jwks.json')),
            ]);
        $cache = new JwksCache(new Client(['handler' => HandlerStack::create($mock)]), new Repository(new ArrayStore()));
        $ts = strtotime($now);
        $verifier = new FieldTokenVerifier($cache, $f['issuer'], $f['appCode'], now: static fn (): int => $ts);
        $this->app->instance(FieldTokenVerifier::class, $verifier);
        Auth::forgetGuards();
    }

    /** @param array<string, mixed> $case */
    private function send(array $case, string $scheme = 'DPoP')
    {
        return $this->call('POST', '/api/orders' . (str_contains($case['url'], '?') ? '?' . explode('?', $case['url'])[1] : ''), [], [], [], [
            'HTTP_AUTHORIZATION' => $scheme . ' ' . $case['accessToken'],
            'HTTP_DPOP' => $case['dpopProof'],
        ]);
    }

    public function test_every_fixture_through_the_middleware(): void
    {
        foreach ($this->fixtures['cases'] as $case) {
            $this->useVerifier($case['now']);
            $res = $this->send($case);
            $name = $case['name'];
            if ($case['expect'] === 'valid') {
                $res->assertOk();
                $this->assertSame(['worker' => 'W001', 'viaGuard' => 'fwu_0FIXTUREWORKER', 'viaRequest' => 'fwu_0FIXTUREWORKER'], $res->json(), $name);
            } elseif ($case['expect'] === 'pcr') {
                $res->assertStatus(403);
                $this->assertSame(['error' => 'PIN_CHANGE_REQUIRED'], $res->json(), $name);
            } else {
                $res->assertStatus(401);
                $this->assertSame(['error' => $case['expect']], $res->json(), $name);
                $this->assertSame('DPoP error="invalid_token"', $res->headers->get('WWW-Authenticate'), $name);
            }
        }
    }

    public function test_guard_user_is_a_field_principal_and_refusal_is_recorded(): void
    {
        $valid = $this->fixtures['cases'][0];
        $this->useVerifier($valid['now']);
        $this->send($valid)->assertOk();

        $this->useVerifier($valid['now']);
        $this->send($valid, 'Bearer')->assertStatus(401);
        $this->assertSame('malformed', json_decode($this->send($valid, 'Bearer')->getContent(), true)['error']);
    }

    public function test_no_credentials_is_401(): void
    {
        $this->useVerifier($this->fixtures['cases'][0]['now']);
        $res = $this->call('POST', '/api/orders');
        $res->assertStatus(401);
        $this->assertSame('DPoP error="invalid_token"', $res->headers->get('WWW-Authenticate'));
    }

    public function test_key_fetch_failure_is_503_not_401(): void
    {
        $valid = $this->fixtures['cases'][0];
        $this->useVerifier($valid['now'], keysDown: true);
        $this->send($valid)->assertStatus(503);
    }

    public function test_guard_is_configured_without_editing_auth_config(): void
    {
        $this->assertSame(['driver' => 'field'], config('auth.guards.field'));
    }
}
