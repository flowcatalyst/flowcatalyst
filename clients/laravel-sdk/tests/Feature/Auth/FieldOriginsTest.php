<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Feature\Auth;

use FlowCatalyst\Auth\Contracts\FieldClientCheck;
use FlowCatalyst\Auth\Field\FieldGuard;
use FlowCatalyst\Auth\Field\FieldOrigins;
use FlowCatalyst\Auth\Field\FieldPrincipal;
use FlowCatalyst\Auth\Field\FieldTokenException;
use FlowCatalyst\Auth\Field\FieldTokenVerifier;
use FlowCatalyst\FlowCatalystServiceProvider;
use FlowCatalyst\Tests\Support\FieldMint;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Auth;
use Orchestra\Testbench\TestCase;
use PHPUnit\Framework\Attributes\DataProvider;

/**
 * Where a field-worker request may have been made (`app_base_url` as one
 * origin, a list or a wildcard) and the application's `FieldClientCheck`,
 * end to end: real tokens and proofs, the real guard and middleware. The
 * proof's `htu` comparison is the verifier's own; nothing here mocks it.
 */
final class FieldOriginsTest extends TestCase
{
    private FieldMint $mint;

    protected function getPackageProviders($app): array
    {
        return [FlowCatalystServiceProvider::class];
    }

    protected function getEnvironmentSetUp($app): void
    {
        $app['config']->set('app.key', 'base64:' . base64_encode(str_repeat('a', 32)));
        $app['config']->set('flowcatalyst.base_url', FieldMint::ISSUER);
        $app['config']->set('flowcatalyst.field.app_code', FieldMint::APP_CODE);
    }

    protected function defineRoutes($router): void
    {
        $router->post('/api/orders', fn () => ['worker' => request()->user()?->workerCode])->middleware('fc.field');
    }

    protected function setUp(): void
    {
        parent::setUp();
        $this->mint = new FieldMint();
        $this->app->instance(FieldTokenVerifier::class, $this->mint->verifier());
    }

    /** @param string|list<string> $config */
    private function origins(string|array $config, bool $allowAll = true): void
    {
        $this->app['config']->set('flowcatalyst.field.app_base_url', $config);
        // Several origins or a wildcard need a check; an allow-all one keeps these tests on the address.
        if ($allowAll && !FieldOrigins::fromConfig($config)->isFixed()) {
            $this->bindCheck(static fn (): bool => true);
        }
        Auth::forgetGuards();
    }

    /**
     * A POST to $url as a proxy would pass it on: the request's own scheme and
     * port are whatever $url says; the proof is signed for $htu.
     *
     * @param array<string, mixed> $claims
     */
    private function callAs(string $url, string $htu, array $claims = [])
    {
        return $this->call('POST', $url, [], [], [], $this->mint->headers('POST', $htu, $claims));
    }

    private function assertAccepted($res): void
    {
        $res->assertOk();
        $this->assertSame(['worker' => 'W001'], $res->json());
    }

    private function assertRefused($res, string $reason): void
    {
        $res->assertStatus(401);
        $this->assertSame(['error' => $reason], $res->json());
        $this->assertSame('DPoP error="invalid_token"', $res->headers->get('WWW-Authenticate'));
    }

    // --- one origin: today's behaviour --------------------------------------

    public function test_single_origin_ignores_the_request_host(): void
    {
        $this->origins('https://app.example.com');
        // Arrives under another host, scheme and port; the proof is for the configured origin.
        $this->assertAccepted($this->callAs('http://internal.local:8080/api/orders', 'https://app.example.com/api/orders'));
        // A proof for the address the request arrived at is not the configured origin.
        $this->assertRefused($this->callAs('http://internal.local:8080/api/orders', 'http://internal.local:8080/api/orders'), 'htu_mismatch');
    }

    public function test_single_origin_never_reads_the_host(): void
    {
        $this->origins('https://app.example.com');
        $request = new class extends Request {
            public function getHost(): string
            {
                throw new \LogicException('the host was read');
            }
        };
        $request = $request::create('http://x.test/api/orders', 'POST', [], [], [], $this->mint->headers('POST', 'https://app.example.com/api/orders'));
        $this->assertSame('W001', $this->guardFor($request)->user()?->workerCode);
    }

    public function test_single_origin_with_a_path_is_read_as_its_origin(): void
    {
        $this->origins('https://app.example.com/base?x=1#y');
        $this->assertAccepted($this->callAs('http://h.test/api/orders', 'https://app.example.com/api/orders'));
    }

    public function test_the_origin_in_the_environment_variable_is_a_plain_string(): void
    {
        $this->origins('  https://app.example.com  ');
        $this->assertAccepted($this->callAs('http://h.test/api/orders', 'https://app.example.com/api/orders'));
    }

    private function guardFor(Request $request): FieldGuard
    {
        return new FieldGuard(
            $this->app->make(FieldTokenVerifier::class),
            static fn () => $request,
            $this->app['config']['flowcatalyst.field.app_base_url'],
            fn () => $this->app->bound(FieldClientCheck::class) ? $this->app->make(FieldClientCheck::class) : null,
        );
    }

    // --- a list of exact origins --------------------------------------------

    public function test_a_list_of_exact_origins(): void
    {
        $this->origins(['https://acme.example.com', 'https://globex.example.org:8443']);
        $this->assertAccepted($this->callAs('http://acme.example.com/api/orders', 'https://acme.example.com/api/orders'));
        $this->assertAccepted($this->callAs('http://globex.example.org/api/orders', 'https://globex.example.org:8443/api/orders'));
    }

    public function test_a_proof_for_one_host_sent_to_another_is_refused(): void
    {
        $this->origins(['https://acme.example.com', 'https://globex.example.org']);
        $this->assertRefused($this->callAs('http://globex.example.org/api/orders', 'https://acme.example.com/api/orders'), 'htu_mismatch');
        $this->assertRefused($this->callAs('http://acme.example.com/api/orders', 'https://globex.example.org/api/orders'), 'htu_mismatch');
    }

    public function test_a_host_that_matches_no_entry_is_refused_even_with_its_own_proof(): void
    {
        $this->origins(['https://acme.example.com', 'https://globex.example.org']);
        $this->assertRefused($this->callAs('http://evil.test/api/orders', 'https://evil.test/api/orders'), 'htu_mismatch');
        $this->assertRefused($this->callAs('http://evil.test/api/orders', 'http://evil.test/api/orders'), 'htu_mismatch');
        // Not even a valid token reaches the verifier: the reason is the host's, whatever else is wrong.
        $this->assertRefused($this->call('POST', 'http://evil.test/api/orders'), 'htu_mismatch');
    }

    public function test_an_exact_entry_keeps_its_port_and_ignores_the_requests(): void
    {
        $this->origins(['https://acme.example.com:8443', 'https://other.example.com']);
        $this->assertAccepted($this->callAs('http://acme.example.com:80/api/orders', 'https://acme.example.com:8443/api/orders'));
        $this->assertRefused($this->callAs('http://acme.example.com:80/api/orders', 'https://acme.example.com/api/orders'), 'htu_mismatch');
    }

    public function test_a_comma_separated_string_whitespace_and_empty_entries(): void
    {
        $this->origins(' https://acme.example.com ,, https://globex.example.org ,');
        $this->assertAccepted($this->callAs('http://acme.example.com/api/orders', 'https://acme.example.com/api/orders'));
        $this->assertAccepted($this->callAs('http://globex.example.org/api/orders', 'https://globex.example.org/api/orders'));
        $this->assertRefused($this->callAs('http://other.example.org/api/orders', 'https://other.example.org/api/orders'), 'htu_mismatch');

        $this->origins(['', ' https://acme.example.com,https://globex.example.org ', '  ']);
        $this->assertAccepted($this->callAs('http://globex.example.org/api/orders', 'https://globex.example.org/api/orders'));
    }

    // --- wildcard -------------------------------------------------------------

    public function test_a_wildcard_matches_one_label(): void
    {
        $this->origins('https://*.example.com');
        $this->assertAccepted($this->callAs('http://acme.example.com/api/orders', 'https://acme.example.com/api/orders'));
        $this->assertAccepted($this->callAs('http://globex.example.com/api/orders', 'https://globex.example.com/api/orders'));
    }

    /** @return array<string, array{0: string}> */
    public static function hostsAWildcardRefuses(): array
    {
        return [
            'two labels' => ['a.b.example.com'],
            'the bare zone' => ['example.com'],
            'another zone' => ['acme.example.org'],
            'a longer name that ends the same' => ['evilexample.com'],
            'the zone inside a longer one' => ['acme.example.com.evil.test'],
            'a dash-joined look-alike' => ['acme-example.com'],
        ];
    }

    #[DataProvider('hostsAWildcardRefuses')]
    public function test_a_wildcard_does_not_match(string $host): void
    {
        $this->origins('https://*.example.com');
        $this->assertRefused($this->callAs("http://{$host}/api/orders", "https://{$host}/api/orders"), 'htu_mismatch');
    }

    public function test_host_case_and_a_trailing_dot_are_handled(): void
    {
        $this->origins('https://*.example.com');
        $this->assertAccepted($this->callAs('http://AcMe.Example.COM/api/orders', 'https://acme.example.com/api/orders'));
        $this->assertAccepted($this->callAs('http://acme.example.com./api/orders', 'https://acme.example.com/api/orders'));
        // A dot after a bare zone is still the bare zone, and a dot alone is no host.
        $this->assertRefused($this->callAs('http://example.com./api/orders', 'https://example.com/api/orders'), 'htu_mismatch');
        $this->origins('https://*.EXAMPLE.com.');
        $this->assertAccepted($this->callAs('http://acme.example.com/api/orders', 'https://acme.example.com/api/orders'));
    }

    public function test_a_wildcard_uses_the_entrys_scheme_and_port_not_the_requests(): void
    {
        $this->origins('https://*.example.com');
        // Behind a proxy the request is http on port 80; the device called https.
        $this->assertAccepted($this->callAs('http://acme.example.com:80/api/orders', 'https://acme.example.com/api/orders'));
        $this->assertRefused($this->callAs('http://acme.example.com/api/orders', 'http://acme.example.com/api/orders'), 'htu_mismatch');

        $this->origins('https://*.example.com:8443');
        $this->assertAccepted($this->callAs('http://acme.example.com/api/orders', 'https://acme.example.com:8443/api/orders'));
        $this->assertRefused($this->callAs('http://acme.example.com/api/orders', 'https://acme.example.com/api/orders'), 'htu_mismatch');
        $this->assertRefused($this->callAs('https://acme.example.com:8443/api/orders', 'https://acme.example.com/api/orders'), 'htu_mismatch');
    }

    public function test_first_matching_entry_wins_and_exact_and_wildcard_mix(): void
    {
        $this->origins('https://www.example.com:9000, https://*.example.com');
        $this->assertAccepted($this->callAs('http://www.example.com/api/orders', 'https://www.example.com:9000/api/orders'));
        $this->assertAccepted($this->callAs('http://acme.example.com/api/orders', 'https://acme.example.com/api/orders'));
    }

    /** Laravel's own request handling already answers 400 to a malformed Host; the guard must not 500 if one reaches it. */
    public function test_a_malformed_host_is_refused_by_the_guard_itself(): void
    {
        $this->origins('https://*.example.com');
        $request = Request::create('http://acme.example.com/api/orders', 'POST', [], [], [], $this->mint->headers('POST', 'https://acme.example.com/api/orders'));
        $request->headers->set('Host', 'bad host!');
        $guard = $this->guardFor($request);
        $this->assertNull($guard->user());
        $this->assertSame('htu_mismatch', $guard->refusal()?->reason);
    }

    // --- configuration errors ---------------------------------------------------

    /** @return array<string, array{0: string|list<string>|null}> */
    public static function invalidConfig(): array
    {
        return [
            'nothing' => [''],
            'null' => [null],
            'only commas and spaces' => [' , ,'],
            'an empty list' => [[]],
            'no scheme' => ['app.example.com'],
            'no host' => ['https://'],
            'no scheme, in a list' => [['https://acme.example.com', 'globex.example.org']],
            'a wildcard for a top-level name' => ['https://*.com'],
            'a wildcard alone' => ['https://*'],
            'a wildcard inside a label' => ['https://a*.example.com'],
            'a wildcard that ends a label' => ['https://*a.example.com'],
            'a wildcard in the middle' => ['https://acme.*.example.com'],
            'a wildcard on the right' => ['https://acme.example.*'],
            'two wildcards' => ['https://*.*.example.com'],
            'an empty label after the wildcard' => ['https://*..example.com'],
            'a bad entry after a good one' => ['https://acme.example.com, https://*.com'],
        ];
    }

    /** @param string|list<string>|null $config */
    #[DataProvider('invalidConfig')]
    public function test_invalid_configuration_is_a_logic_exception(string|array|null $config): void
    {
        $this->expectException(\LogicException::class);
        FieldOrigins::fromConfig($config);
    }

    public function test_invalid_configuration_is_a_loud_error_at_the_middleware(): void
    {
        $this->app['config']->set('flowcatalyst.field.app_base_url', 'https://*.com');
        Auth::forgetGuards();
        $this->withoutExceptionHandling();
        $this->expectException(\LogicException::class);
        $this->callAs('http://acme.com/api/orders', 'https://acme.com/api/orders');
    }

    public function test_a_missing_origin_keeps_its_message(): void
    {
        $this->expectException(\LogicException::class);
        $this->expectExceptionMessage('The field guard needs flowcatalyst.field.app_base_url (or APP_URL)');
        FieldOrigins::fromConfig('');
    }

    public function test_a_path_query_and_fragment_on_an_entry_are_ignored(): void
    {
        $this->origins(['https://*.example.com/some/path?q=1#f']);
        $this->assertAccepted($this->callAs('http://acme.example.com/api/orders', 'https://acme.example.com/api/orders'));
    }

    // --- the client check -------------------------------------------------------

    private function bindCheck(\Closure $allows): object
    {
        $check = new class ($allows) implements FieldClientCheck {
            public int $calls = 0;
            /** @var list<string> */
            public array $seen = [];

            public function __construct(private readonly \Closure $allows) {}

            public function allows(FieldPrincipal $worker, Request $request): bool
            {
                $this->calls++;
                $this->seen[] = $worker->clientIdentifier . '@' . $request->getHost();
                return ($this->allows)($worker, $request);
            }
        };
        $this->app->instance(FieldClientCheck::class, $check);
        return $check;
    }

    public function test_one_fixed_origin_needs_no_check_and_allows_every_verified_worker(): void
    {
        $this->origins('https://acme.example.com');
        $this->assertFalse($this->app->bound(FieldClientCheck::class));
        $this->assertAccepted($this->callAs('http://globex.example.org/api/orders', 'https://acme.example.com/api/orders'));
    }

    public function test_one_fixed_origin_honours_a_check_that_is_bound(): void
    {
        $this->origins('https://acme.example.com');
        $this->bindCheck(static fn (): bool => false);
        $this->callAs('http://x.test/api/orders', 'https://acme.example.com/api/orders')->assertStatus(403);
    }

    /** @return array<string, array{0: string|list<string>}> */
    public static function nonFixedConfig(): array
    {
        return [
            'a list' => [['https://acme.example.com', 'https://globex.example.org']],
            'a comma-separated string' => ['https://acme.example.com,https://globex.example.org'],
            'a wildcard' => ['https://*.example.com'],
            'a list of one wildcard' => [['https://*.example.com']],
        ];
    }

    /** @param string|list<string> $config */
    #[DataProvider('nonFixedConfig')]
    public function test_several_origins_or_a_wildcard_without_a_check_is_a_logic_exception_in_the_guard(string|array $config): void
    {
        $this->origins($config, allowAll: false);
        $request = Request::create('http://acme.example.com/api/orders', 'POST', [], [], [], $this->mint->headers('POST', 'https://acme.example.com/api/orders'));
        $guard = $this->guardFor($request);
        foreach ([1, 2] as $_) { // every call raises it, none is quietly a refusal
            try {
                $guard->user();
                $this->fail('no LogicException');
            } catch (\LogicException $e) {
                $this->assertStringContainsString('FlowCatalyst\\Auth\\Contracts\\FieldClientCheck', $e->getMessage());
                $this->assertStringContainsString('returns true', $e->getMessage());
            }
        }
    }

    /** @param string|list<string> $config */
    #[DataProvider('nonFixedConfig')]
    public function test_several_origins_or_a_wildcard_without_a_check_is_a_500_at_the_middleware_whatever_the_host(string|array $config): void
    {
        $this->origins($config, allowAll: false);
        $this->withoutExceptionHandling();
        foreach (['acme.example.com', 'evil.test'] as $host) {
            try {
                $this->callAs("http://{$host}/api/orders", "https://{$host}/api/orders");
                $this->fail('no LogicException');
            } catch (\LogicException $e) {
                $this->assertStringContainsString('FieldClientCheck', $e->getMessage());
            }
        }
        // even with no credentials at all: it is thrown before the token is read
        try {
            $this->call('POST', 'http://acme.example.com/api/orders');
            $this->fail('no LogicException');
        } catch (\LogicException) {
            $this->addToAssertionCount(1);
        }
    }

    public function test_a_bad_entry_is_reported_before_the_missing_check(): void
    {
        $this->app['config']->set('flowcatalyst.field.app_base_url', 'https://acme.example.com, https://*.com');
        Auth::forgetGuards();
        $request = Request::create('http://acme.example.com/api/orders', 'POST');
        try {
            $this->guardFor($request)->user();
            $this->fail('no LogicException');
        } catch (\LogicException $e) {
            $this->assertStringContainsString('https://*.com', $e->getMessage());
            $this->assertStringNotContainsString('FieldClientCheck', $e->getMessage());
        }
    }

    public function test_an_unknown_host_with_a_check_bound_is_still_htu_mismatch(): void
    {
        $this->origins(['https://acme.example.com', 'https://globex.example.org']);
        $this->assertRefused($this->callAs('http://evil.test/api/orders', 'https://evil.test/api/orders'), 'htu_mismatch');
    }

    public function test_a_check_that_allows_gives_the_principal(): void
    {
        $this->origins('https://*.example.com');
        $check = $this->bindCheck(static fn (FieldPrincipal $w, Request $r): bool => $w->clientIdentifier === explode('.', $r->getHost())[0]);
        $this->assertAccepted($this->callAs('http://acme.example.com/api/orders', 'https://acme.example.com/api/orders'));
        $this->assertSame(['acme@acme.example.com'], $check->seen);
    }

    public function test_a_check_that_refuses_is_403_wrong_client_without_www_authenticate(): void
    {
        $this->origins('https://*.example.com');
        $this->bindCheck(static fn (FieldPrincipal $w, Request $r): bool => $w->clientIdentifier === explode('.', $r->getHost())[0]);
        $res = $this->callAs('http://globex.example.com/api/orders', 'https://globex.example.com/api/orders');
        $res->assertStatus(403);
        $this->assertSame(['error' => 'WRONG_CLIENT'], $res->json());
        $this->assertNull($res->headers->get('WWW-Authenticate'));
        $this->assertFalse($res->headers->has('WWW-Authenticate'));
    }

    public function test_the_guard_gives_no_user_and_the_reason_wrong_client(): void
    {
        $this->origins('https://acme.example.com');
        $this->bindCheck(static fn (): bool => false);
        $request = Request::create('http://x.test/api/orders', 'POST', [], [], [], $this->mint->headers('POST', 'https://acme.example.com/api/orders'));
        $guard = $this->guardFor($request);
        $this->assertNull($guard->user());
        $this->assertSame(FieldTokenException::REASON_WRONG_CLIENT, $guard->refusal()?->reason);
        $this->assertSame('wrong_client', FieldTokenException::REASON_WRONG_CLIENT);
    }

    public function test_the_check_runs_once_per_request(): void
    {
        $this->origins('https://acme.example.com');
        $check = $this->bindCheck(static fn (): bool => true);
        $request = Request::create('http://x.test/api/orders', 'POST', [], [], [], $this->mint->headers('POST', 'https://acme.example.com/api/orders'));
        $guard = $this->guardFor($request);
        $guard->user();
        $guard->user();
        $guard->refusal();
        $this->assertSame(1, $check->calls);

        // The middleware and the route's own call to the guard share the one run.
        $res = $this->callAs('http://x.test/api/orders', 'https://acme.example.com/api/orders');
        $this->assertAccepted($res);
        $this->assertSame(2, $check->calls); // one more, for the second request
    }

    public function test_the_check_is_not_called_when_the_credentials_are_refused(): void
    {
        $this->origins('https://acme.example.com');
        $check = $this->bindCheck(static fn (): bool => false);
        // no credentials
        $this->assertRefused($this->call('POST', 'http://acme.example.com/api/orders'), 'malformed');
        // a proof for another address
        $this->assertRefused($this->callAs('http://acme.example.com/api/orders', 'https://acme.example.com/api/elsewhere'), 'htu_mismatch');
        // a token for another application
        $this->assertRefused($this->callAs('http://acme.example.com/api/orders', 'https://acme.example.com/api/orders', ['aud' => 'another-app']), 'wrong_aud');
        // an expired token
        $this->assertRefused($this->callAs('http://acme.example.com/api/orders', 'https://acme.example.com/api/orders', ['exp' => time() - 3600]), 'expired');
        $this->assertSame(0, $check->calls);
    }

    public function test_the_check_is_not_called_for_a_host_that_matches_no_origin(): void
    {
        $this->origins(['https://acme.example.com', 'https://globex.example.org']);
        $check = $this->bindCheck(static fn (): bool => true);
        $this->assertRefused($this->callAs('http://evil.test/api/orders', 'https://evil.test/api/orders'), 'htu_mismatch');
        $this->assertSame(0, $check->calls);
    }

    public function test_a_pin_change_only_token_is_reported_before_the_check_runs(): void
    {
        $this->origins('https://acme.example.com');
        $check = $this->bindCheck(static fn (): bool => false);
        $res = $this->callAs('http://acme.example.com/api/orders', 'https://acme.example.com/api/orders', ['pcr' => true]);
        $res->assertStatus(403);
        $this->assertSame(['error' => 'PIN_CHANGE_REQUIRED'], $res->json());
        $this->assertSame(0, $check->calls);
    }

    public function test_a_check_that_throws_surfaces_as_the_unavailable_answer(): void
    {
        $this->origins('https://acme.example.com');
        $this->bindCheck(static function (): bool {
            throw new \RuntimeException('tenant lookup failed');
        });
        $res = $this->callAs('http://acme.example.com/api/orders', 'https://acme.example.com/api/orders');
        $res->assertStatus(503);
        $this->assertSame(['error' => 'unavailable'], $res->json());
        $this->assertFalse($res->headers->has('WWW-Authenticate'));
    }

    public function test_a_binding_made_after_the_guard_was_built_is_seen(): void
    {
        $this->origins('https://acme.example.com', allowAll: false);
        // The guard is built and used first, with no check bound.
        $this->assertAccepted($this->callAs('http://x.test/api/orders', 'https://acme.example.com/api/orders'));
        $this->bindCheck(static fn (): bool => false);
        $this->callAs('http://x.test/api/orders', 'https://acme.example.com/api/orders')->assertStatus(403);
    }

    public function test_the_verifier_alone_does_not_run_the_check(): void
    {
        $check = $this->bindCheck(static fn (): bool => false);
        $token = $this->mint->token();
        $principal = $this->mint->verifier()->verify('DPoP ' . $token, $this->mint->proof($token, 'POST', 'https://acme.example.com/x'), 'POST', 'https://acme.example.com/x');
        $this->assertSame('W001', $principal->workerCode);
        $this->assertSame(0, $check->calls);
    }
}
