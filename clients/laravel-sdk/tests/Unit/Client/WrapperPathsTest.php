<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Unit\Client;

use FlowCatalyst\Client\Auth\TokenProviderInterface;
use FlowCatalyst\Client\FlowCatalystClient;
use GuzzleHttp\Client;
use GuzzleHttp\HandlerStack;
use GuzzleHttp\Middleware;
use GuzzleHttp\Promise\Create;
use GuzzleHttp\Psr7\Response;
use PHPUnit\Framework\TestCase;
use Psr\Http\Message\RequestInterface;

/**
 * Every request a hand-written resource method sends must be an operation of
 * the spec this package carries (openapi/openapi.json and its app and admin
 * splits). A method that calls a path or method the platform does not have can
 * only ever fail (getServiceAccount called GET /api/applications/{id}/service-account
 * before the platform had it; updateClientConfig a PUT that never existed).
 *
 * The test calls every public method of every class in src/Client/Resources
 * with placeholder arguments against a stub that answers 200 `{}` to
 * everything, and checks what was sent, so a new method is covered without a
 * test of its own. A method that fails before sending (it validates its
 * arguments first) sends nothing and is skipped; the minimum below keeps the
 * test from passing on nothing.
 */
final class WrapperPathsTest extends TestCase
{
    /** @var array<string, list<string>> normalised path => methods */
    private array $operations = [];

    protected function setUp(): void
    {
        foreach (['openapi.json', 'openapi.app.json', 'openapi.admin.json'] as $file) {
            $spec = json_decode((string) file_get_contents(__DIR__ . '/../../../openapi/' . $file), true, flags: JSON_THROW_ON_ERROR);
            foreach ($spec['paths'] as $path => $item) {
                foreach (array_keys($item) as $method) {
                    if (in_array($method, ['get', 'post', 'put', 'delete', 'patch', 'head'], true)) {
                        $this->operations[self::normalise($path)][] = strtoupper($method);
                    }
                }
            }
        }
    }

    private static function normalise(string $path): string
    {
        $path = rawurldecode(explode('?', $path, 2)[0]);
        $segments = array_map(
            // A spec template ({id}) and a placeholder argument (x-1, 7001) are both a parameter.
            static fn(string $s): string => preg_match('/^\{.*\}$|^x-\d+$|^7\d{3}$/', $s) ? '{}' : $s,
            explode('/', $path),
        );

        return rtrim(implode('/', $segments), '/');
    }

    public function test_every_request_a_resource_method_sends_is_in_the_spec(): void
    {
        $sent = new \ArrayObject();
        $stack = HandlerStack::create(static fn() => Create::promiseFor(
            new Response(200, ['Content-Type' => 'application/json'], '{}')
        ));
        $stack->push(Middleware::history($sent));
        $client = new FlowCatalystClient(
            tokenProvider: new class implements TokenProviderInterface {
                public function getAccessToken(): string { return 'tok'; }
                public function refreshToken(): string { return 'tok'; }
            },
            baseUrl: 'https://fc.test',
            retryAttempts: 1,
            retryDelay: 1,
        );
        $prop = new \ReflectionProperty(FlowCatalystClient::class, 'httpClient');
        $prop->setValue($client, new Client(['handler' => $stack, 'http_errors' => false, 'base_uri' => 'https://fc.test']));

        $problems = [];
        $seen = [];
        $called = 0;
        foreach (glob(__DIR__ . '/../../../src/Client/Resources/*.php') as $file) {
            $class = 'FlowCatalyst\\Client\\Resources\\' . basename($file, '.php');
            $resource = new $class($client);
            if (method_exists($resource, 'viaAdmin')) {
                $resource = $resource->viaAdmin();
            }
            foreach ((new \ReflectionClass($class))->getMethods(\ReflectionMethod::IS_PUBLIC) as $method) {
                if ($method->isConstructor() || $method->isStatic() || $method->getName() === 'viaAdmin'
                    || $method->getDeclaringClass()->getName() !== $class) {
                    continue;
                }
                $args = $this->placeholders($method);
                if ($args === null) {
                    continue;
                }
                $before = count($sent);
                // The stub's `{}` rarely maps onto a DTO (missing keys, wrong
                // types); only the request matters, so the mapping's notices
                // and exceptions are dropped.
                set_error_handler(static fn() => true);
                try {
                    $method->invokeArgs($resource, $args);
                } catch (\Throwable) {
                } finally {
                    restore_error_handler();
                }
                $entries = array_slice(iterator_to_array($sent), $before);
                if ($entries !== []) {
                    $called++;
                }
                foreach ($entries as $entry) {
                    /** @var RequestInterface $request */
                    $request = $entry['request'];
                    if ($request->getUri()->getHost() !== 'fc.test') {
                        continue; // the router's monitoring host
                    }
                    $verb = $request->getMethod();
                    $path = self::normalise($request->getUri()->getPath());
                    $seen["{$verb} {$path}"] = true;
                    $where = basename($file, '.php') . '::' . $method->getName();
                    if (!isset($this->operations[$path])) {
                        $problems[] = "{$where}: {$verb} {$request->getUri()->getPath()}: no such path";
                    } elseif (!in_array($verb, $this->operations[$path], true)) {
                        $problems[] = "{$where}: {$verb} {$request->getUri()->getPath()}: the path has only "
                            . implode(', ', array_unique($this->operations[$path]));
                    }
                }
            }
        }

        $this->assertSame([], $problems);
        $this->assertGreaterThan(100, $called, "only {$called} methods sent a request");
        foreach ([
            'GET /api/applications/{}/service-account',
            'POST /api/applications/{}/provision-service-account',
            'PUT /api/config/{}/{}/{}',
            'POST /api/ingestions/{}/versions/{}/live',
            'POST /api/applications/{}/roles/sync',
        ] as $known) {
            $this->assertArrayHasKey($known, $seen, "the test no longer reaches {$known}");
        }
    }

    /**
     * Placeholder arguments: distinct strings (`x-1`, `x-2`) and integers
     * (`7001`), read back as path parameters; defaults where there are;
     * objects built the same way.
     *
     * @return list<mixed>|null null when an argument cannot be made
     */
    private function placeholders(\ReflectionFunctionAbstract $fn, int $depth = 0): ?array
    {
        static $n = 0;
        $args = [];
        foreach ($fn->getParameters() as $param) {
            if ($param->isVariadic()) {
                break;
            }
            if ($param->isDefaultValueAvailable() && !$this->isClassType($param->getType())) {
                $args[] = $param->getDefaultValue();
                continue;
            }
            $value = $this->placeholder($param->getType(), $depth, $n);
            if ($value === false && !($param->getType() instanceof \ReflectionNamedType && $param->getType()->getName() === 'bool')) {
                if ($param->allowsNull()) {
                    $args[] = null;
                    continue;
                }

                return null;
            }
            $args[] = $value;
        }

        return $args;
    }

    private function isClassType(?\ReflectionType $type): bool
    {
        return $type instanceof \ReflectionNamedType && !$type->isBuiltin();
    }

    /** @return mixed false when no placeholder can be made */
    private function placeholder(?\ReflectionType $type, int $depth, int &$n): mixed
    {
        if ($type instanceof \ReflectionUnionType) {
            $type = $type->getTypes()[0];
        }
        if (!$type instanceof \ReflectionNamedType) {
            return 'x-' . ++$n;
        }
        $name = $type->getName();
        switch ($name) {
            case 'string': return 'x-' . ++$n;
            case 'int': return 7000 + ++$n;
            case 'float': return 1.0;
            case 'bool': return false;
            case 'array': return [];
            case 'mixed': return null;
        }
        if ($depth > 3 || !class_exists($name) && !enum_exists($name)) {
            return false;
        }
        if (enum_exists($name)) {
            return $name::cases()[0] ?? false;
        }
        $class = new \ReflectionClass($name);
        if (!$class->isInstantiable()) {
            return false;
        }
        $ctor = $class->getConstructor();
        $args = $ctor === null ? [] : $this->placeholders($ctor, $depth + 1);
        if ($args === null) {
            return false;
        }
        try {
            return $class->newInstanceArgs($args);
        } catch (\Throwable) {
            return false;
        }
    }
}
