<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Unit\Client;

use FlowCatalyst\Client\Admin;
use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\Client\Resources\Router;
use FlowCatalyst\DTOs\Requests\ClientConfigRequest;
use FlowCatalyst\DTOs\Requests\CreateApplicationRequest;
use FlowCatalyst\DTOs\Requests\CreateClientRequest;
use FlowCatalyst\DTOs\Requests\CreateUserRequest;
use FlowCatalyst\DTOs\Requests\UpdateApplicationRequest;
use FlowCatalyst\DTOs\Requests\UpdateClientApplicationsRequest;
use FlowCatalyst\DTOs\Requests\UpdateClientRequest;
use FlowCatalyst\DTOs\Requests\UpdatePrincipalRequest;
use FlowCatalyst\Support\Deprecations;
use GuzzleHttp\Client as Guzzle;
use GuzzleHttp\Handler\MockHandler;
use GuzzleHttp\HandlerStack;
use GuzzleHttp\Middleware;
use GuzzleHttp\Psr7\Response;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

/**
 * `$client->admin()` delegates to the same resource classes as the legacy
 * accessors. Requests are captured at FlowCatalystClient::request(); the router
 * (its own HTTP client) is exercised through a Guzzle mock handler. No test
 * touches the network.
 */
final class AdminFacadeTest extends TestCase
{
    /** @var array<int, array{0: string, 1: string, 2: array}> */
    private array $calls = [];

    /** @var list<string> */
    private array $deprecations = [];

    /** @var callable|null */
    private $previousHandler = null;

    private int $previousReporting = 0;

    protected function setUp(): void
    {
        $this->previousReporting = error_reporting(E_ALL);
        Deprecations::reset();
        $this->calls = [];
        $this->deprecations = [];
        $this->previousHandler = set_error_handler(function (int $no, string $str): bool {
            if ($no === E_USER_DEPRECATED && (error_reporting() & $no)) {
                $this->deprecations[] = $str;
                return true;
            }
            return false;
        });
    }

    protected function tearDown(): void
    {
        restore_error_handler();
        error_reporting($this->previousReporting);
        Deprecations::reset();
    }

    /** A request-capturing client whose real accessors and admin() are intact. */
    private function client(?array $answer = null): FlowCatalystClient
    {
        $client = $this->getMockBuilder(FlowCatalystClient::class)
            ->disableOriginalConstructor()
            ->onlyMethods(['request'])
            ->getMock();
        $client->method('request')->willReturnCallback(
            function (string $method, string $endpoint, array $options = []) use ($answer) {
                $this->calls[] = [$method, $endpoint, $options];
                return $answer ?? [
                    'id' => 'id_1', 'type' => 'USER', 'name' => 'N', 'identifier' => 'ident',
                    'code' => 'c', 'applicationId' => 'a1', 'email' => 'a@b.c', 'clientId' => 'clt_1',
                    'principals' => [['id' => 'p1', 'type' => 'USER', 'name' => 'A', 'email' => 'a@b.c']],
                    'clients' => [['id' => 'c1', 'name' => 'C', 'identifier' => 'ci']],
                    'applications' => [], 'roles' => [], 'grants' => [], 'clientConfigs' => [],
                    'items' => [], 'total' => 1, 'enabled' => true,
                ];
            }
        );

        return $client;
    }

    /** @return array<string, array{0: string, 1: string, 2: callable}> */
    public static function movedMethods(): array
    {
        $cases = [
            ['principals', 'list', fn() => ['active' => true, 'type' => 'USER']],
            ['principals', 'get', fn() => ['p1']],
            ['principals', 'findByEmail', fn() => ['a@b.c']],
            ['principals', 'createUser', fn() => [new CreateUserRequest(email: 'a@b.c', name: 'A')]],
            ['principals', 'resetPassword', fn() => ['p1', 'pw', false]],
            ['principals', 'update', fn() => ['p1', new UpdatePrincipalRequest(name: 'B')]],
            ['principals', 'activate', fn() => ['p1']],
            ['principals', 'deactivate', fn() => ['p1']],
            ['principals', 'getRoles', fn() => ['p1']],
            ['principals', 'addRole', fn() => ['p1', 'app:role']],
            ['principals', 'removeRole', fn() => ['p1', 'app:role']],
            ['principals', 'setRoles', fn() => ['p1', ['app:role']]],
            ['principals', 'getClientAccessGrants', fn() => ['p1']],
            ['principals', 'grantClientAccess', fn() => ['p1', 'clt_1']],
            ['principals', 'revokeClientAccess', fn() => ['p1', 'clt_1']],
            ['clients', 'search', fn() => ['acme']],
            ['clients', 'create', fn() => [new CreateClientRequest(identifier: 'acme', name: 'Acme')]],
            ['clients', 'update', fn() => ['c1', new UpdateClientRequest(name: 'X')]],
            ['clients', 'activate', fn() => ['c1']],
            ['clients', 'suspend', fn() => ['c1', 'why']],
            ['clients', 'deactivate', fn() => ['c1', 'why']],
            ['clients', 'addNote', fn() => ['c1', 'cat', 'text']],
            ['clients', 'getApplications', fn() => ['c1']],
            ['clients', 'updateApplications', fn() => ['c1', new UpdateClientApplicationsRequest(['a1'])]],
            ['clients', 'enableApplication', fn() => ['c1', 'a1']],
            ['clients', 'disableApplication', fn() => ['c1', 'a1']],
            ['applications', 'list', fn() => [true]],
            ['applications', 'create', fn() => [new CreateApplicationRequest(code: 'app', name: 'App')]],
            ['applications', 'update', fn() => ['a1', new UpdateApplicationRequest(name: 'X')]],
            ['applications', 'delete', fn() => ['a1']],
            ['applications', 'activate', fn() => ['a1']],
            ['applications', 'deactivate', fn() => ['a1']],
            ['applications', 'provisionServiceAccount', fn() => ['a1']],
            ['applications', 'getServiceAccount', fn() => ['a1']],
            ['applications', 'listClients', fn() => ['a1']],
            ['applications', 'updateClientConfig', fn() => ['a1', 'c1', new ClientConfigRequest(enabled: true)]],
            ['applications', 'enableForClient', fn() => ['a1', 'c1']],
            ['applications', 'disableForClient', fn() => ['a1', 'c1']],
        ];

        $out = [];
        foreach ($cases as [$accessor, $method, $args]) {
            $out["{$accessor}::{$method}"] = [$accessor, $method, $args];
        }

        return $out;
    }

    /** @return array{0: mixed, 1: array} outcome (value or exception) and captured calls */
    private function invoke(FlowCatalystClient $client, object $resource, string $method, array $args): array
    {
        $this->calls = [];
        try {
            $outcome = $resource->{$method}(...$args);
        } catch (\Throwable $e) {
            $outcome = [get_class($e), $e->getMessage()];
        }

        return [$outcome, $this->calls];
    }

    #[DataProvider('movedMethods')]
    public function test_admin_matches_legacy_accessor(string $accessor, string $method, callable $args): void
    {
        $client = $this->client();

        $legacy = $this->invoke($client, $client->{$accessor}(), $method, $args());
        $admin = $this->invoke($client, $client->admin()->{$accessor}(), $method, $args());

        $this->assertNotEmpty($legacy[1], 'the call must reach the transport');
        $this->assertEquals($legacy, $admin);
    }

    #[DataProvider('movedMethods')]
    public function test_legacy_path_deprecates_once_per_method_and_admin_never(string $accessor, string $method, callable $args): void
    {
        $client = $this->client();

        $client->admin()->{$accessor}()->{$method}(...$args());
        $client->admin()->{$accessor}()->{$method}(...$args());
        $this->assertSame([], $this->deprecations, 'admin() must never notify');

        $client->{$accessor}()->{$method}(...$args());
        $client->{$accessor}()->{$method}(...$args());
        $client->{$accessor}()->{$method}(...$args());

        $this->assertCount(1, $this->deprecations);
        $this->assertStringContainsString("{$method}()", $this->deprecations[0]);
        $this->assertStringContainsString("admin()->{$accessor}()->{$method}()", $this->deprecations[0]);
    }

    public function test_different_methods_each_notify_once_even_across_client_instances(): void
    {
        $a = $this->client();
        $b = $this->client();

        $a->principals()->get('p1');
        $a->principals()->activate('p1');
        $b->principals()->get('p1'); // once per process, not per client

        $this->assertCount(2, $this->deprecations);
    }

    public function test_find_by_email_does_not_leak_a_list_notice(): void
    {
        $client = $this->client();

        $client->principals()->findByEmail('a@b.c');

        $this->assertCount(1, $this->deprecations);
        $this->assertStringContainsString('findByEmail', $this->deprecations[0]);
        $this->assertStringNotContainsString('::list()', $this->deprecations[0]);
    }

    public function test_methods_that_stay_are_not_deprecated(): void
    {
        $client = $this->client();

        $client->clients()->list();
        $client->clients()->get('c1');
        $client->clients()->getRaw('c1');
        $client->clients()->getByIdentifier('ci');
        $client->applications()->get('a1');
        $client->applications()->getByCode('app');
        $this->client([])->applications()->listRoles('a1');
        $client->principals()->syncUsers([]);
        $client->principals()->setClientAssociation('p1', 'clt_1', 'CHANGE_CLIENT');
        $client->roles()->list();

        $this->assertSame([], $this->deprecations);
    }

    public function test_admin_is_memoised_and_exposes_the_resources(): void
    {
        $client = $this->client();

        $this->assertInstanceOf(Admin::class, $client->admin());
        $this->assertSame($client->admin(), $client->admin());
        $this->assertSame($client->admin()->principals(), $client->admin()->principals());
        $this->assertNotSame($client->principals(), $client->admin()->principals());
    }

    public function test_admin_exposes_set_client_association_and_stays_quiet(): void
    {
        $client = $this->client();

        $client->admin()->principals()->setClientAssociation('p1', 'clt_1', 'TO_PARTNER');

        $this->assertSame(
            [['PUT', '/api/principals/p1/client-association', ['json' => ['clientId' => 'clt_1', 'mode' => 'TO_PARTNER']]]],
            $this->calls
        );
        $this->assertSame([], $this->deprecations);
    }

    public function test_router_admin_matches_legacy_and_notifies_only_through_legacy(): void
    {
        $client = $this->client();

        $inject = function (Router $router): void {
            $responses = [];
            for ($i = 0; $i < 3; $i++) {
                $responses[] = new Response(200, [], json_encode(['messageId' => 'm1', 'inPipeline' => true]));
                $responses[] = new Response(200, [], json_encode(['m1' => true, 'm2' => false]));
            }
            $http = new Guzzle([
                'handler' => HandlerStack::create(new MockHandler($responses)),
                'base_uri' => 'http://router.invalid',
                'http_errors' => false,
            ]);
            (function () use ($http) {
                $this->httpClient = $http;
            })->call($router);
        };

        $inject($client->admin()->router());
        $viaAdmin = [
            $client->admin()->router()->inPipeline('m1'),
            $client->admin()->router()->inPipelineBatch(['m1', 'm2']),
        ];
        $this->assertSame([], $this->deprecations);

        $inject($client->router());
        $legacy = [
            $client->router()->inPipeline('m1'),
            $client->router()->inPipelineBatch(['m1', 'm2']),
        ];
        $client->router()->inPipeline('m1');
        $client->router()->inPipelineBatch(['m1']);

        $this->assertSame($legacy, $viaAdmin);
        $this->assertSame(['m1' => true, 'm2' => false], $legacy[1]);
        $this->assertCount(2, $this->deprecations); // one per method, not per call
    }

    public function test_deprecations_can_be_promoted_to_errors_only_when_a_test_opts_in(): void
    {
        $client = $this->client();

        // Default: the old accessor works and does not throw.
        $this->assertNotNull($client->principals()->get('p1'));

        // Opt in: a handler that throws on E_USER_DEPRECATED sees the notice
        // from a not-yet-notified method, while admin() stays clean.
        restore_error_handler();
        set_error_handler(static function (int $no, string $str): never {
            throw new \ErrorException($str, 0, $no);
        });
        try {
            $client->admin()->principals()->activate('p1');
            $this->expectException(\ErrorException::class);
            $client->principals()->activate('p1');
        } finally {
            restore_error_handler();
            set_error_handler(static fn(): bool => true);
        }
    }

    public function test_notice_is_suppressible(): void
    {
        $client = $this->client();

        $previous = error_reporting(error_reporting() & ~E_USER_DEPRECATED);
        try {
            $client->principals()->deactivate('p1');
        } finally {
            error_reporting($previous);
        }

        // Handler was not invoked for a masked level, and the call succeeded.
        $this->assertSame([], $this->deprecations);
        $this->assertNotEmpty($this->calls);
    }
}
