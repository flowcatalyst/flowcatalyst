<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Unit\Definition;

use FlowCatalyst\Attributes\AsEventType;
use FlowCatalyst\Console\Commands\SyncDefinitionsCommand;
use FlowCatalyst\Definition\DefinitionScanner;
use FlowCatalyst\Sync\SyncDefinitionSet;
use Illuminate\Config\Repository;
use Illuminate\Container\Container;
use PHPUnit\Framework\TestCase;
use ReflectionMethod;

/**
 * Multi-application resolution: a definition's application code is resolved
 * from (1) the attribute `application:` override, then (2) a longest-prefix
 * match in `definitions.application_map`, then (3) null (the sync command's
 * default). The sync command then groups definitions per application.
 */
final class ApplicationResolutionTest extends TestCase
{
    protected function setUp(): void
    {
        $container = new Container();
        Container::setInstance($container);
        $container->instance('config', new Repository([
            'flowcatalyst' => ['definitions' => ['application_map' => [
                'Acme\\Orders\\' => 'orders',
                'Acme\\'         => 'acme', // shorter prefix; longest must win
            ]]],
        ]));
    }

    protected function tearDown(): void
    {
        Container::setInstance(null);
    }

    private function resolve(string $class, AsEventType $instance): ?string
    {
        $m = new ReflectionMethod(DefinitionScanner::class, 'resolveApplication');
        $m->setAccessible(true);
        return $m->invoke(new DefinitionScanner(), $class, $instance);
    }

    public function test_namespace_map_resolves_longest_prefix(): void
    {
        $et = new AsEventType(subdomain: 'orders', aggregate: 'order', event: 'created', name: 'OC');
        $this->assertSame('orders', $this->resolve('Acme\\Orders\\OrderCreated', $et));
        $this->assertSame('acme', $this->resolve('Acme\\Misc\\Thing', $et));
    }

    public function test_attribute_override_beats_map(): void
    {
        $et = new AsEventType(subdomain: 'b', aggregate: 'i', event: 'paid', name: 'IP', application: 'billing');
        $this->assertSame('billing', $this->resolve('Acme\\Orders\\Anything', $et));
    }

    public function test_no_match_resolves_null(): void
    {
        $et = new AsEventType(subdomain: 'x', aggregate: 'y', event: 'z', name: 'X');
        $this->assertNull($this->resolve('App\\Other\\Thing', $et));
    }

    public function test_group_by_application_buckets_by_resolved_app(): void
    {
        $cmd = new SyncDefinitionsCommand();
        $m = new ReflectionMethod($cmd, 'groupByApplication');
        $m->setAccessible(true);

        $data = ['eventTypes' => [
            ['code' => 'a', '_application' => 'orders'],
            ['code' => 'b', '_application' => null],     // → default
            ['code' => 'c', '_application' => 'billing'],
        ]];
        $groups = $m->invoke($cmd, $data, 'example');
        $keys = array_keys($groups);
        sort($keys);

        $this->assertSame(['billing', 'example', 'orders'], $keys);
        $this->assertSame('b', $groups['example']['eventTypes'][0]['code']);
    }

    public function test_internal_fields_are_stripped_from_payload(): void
    {
        $set = SyncDefinitionSet::fromScannedDefinitions('orders', [
            'eventTypes' => [['code' => 'x', '_class' => 'C', '_application' => 'orders']],
        ]);
        $et = $set->getEventTypes()[0];
        $this->assertArrayNotHasKey('_application', $et);
        $this->assertArrayNotHasKey('_class', $et);
        $this->assertSame('x', $et['code']);
    }
}
