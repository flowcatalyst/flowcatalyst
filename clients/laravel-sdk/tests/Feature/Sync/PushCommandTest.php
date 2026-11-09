<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Feature\Sync;

use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\Definition\DefinitionRepository;
use FlowCatalyst\FlowCatalystServiceProvider;
use Illuminate\Support\Facades\Artisan;
use Orchestra\Testbench\TestCase;

/**
 * `flowcatalyst:sync` sends `flowcatalyst.push` (rules + ticket key) to the
 * push gateway for the default application.
 */
final class PushCommandTest extends TestCase
{
    private const SEED = '5BgJptHbkz4uIeRqgLz3Ws_38Ovwi1uCpSdyW45yMdc';

    protected function getPackageProviders($app): array
    {
        return [FlowCatalystServiceProvider::class];
    }

    protected function getEnvironmentSetUp($app): void
    {
        $app['config']->set('flowcatalyst.base_url', 'https://fc.test');
        $app['config']->set('flowcatalyst.application_code', 'shop');
        $app['config']->set('flowcatalyst.definitions.seed_spatie', false);
        $app['config']->set('flowcatalyst.definitions.set_providers', []);
    }

    public function test_configured_rules_and_key_are_synced(): void
    {
        $this->app->make(DefinitionRepository::class)->scanAndCache([]);
        $this->app['config']->set('flowcatalyst.push.rules', [['pattern' => 'shop:board', 'public' => true]]);
        $this->app['config']->set('flowcatalyst.push.ticket_key', 'k1:' . self::SEED);
        $this->app['config']->set('flowcatalyst.push.previous_ticket_keys', '');

        $calls = [];
        $client = $this->createMock(FlowCatalystClient::class);
        $client->method('request')->willReturnCallback(function (string $method, string $path, array $options = []) use (&$calls) {
            $calls[] = [$method, $path, $options];
            return ['applicationCode' => 'shop', 'rules' => ['created' => 1, 'updated' => 0, 'deleted' => 0],
                'keys' => ['created' => 1, 'updated' => 0, 'deleted' => 0], 'syncedPatterns' => ['shop:board'], 'syncedKids' => ['k1']];
        });
        $this->app->instance(FlowCatalystClient::class, $client);

        $exit = Artisan::call('flowcatalyst:sync');
        $out = Artisan::output();

        $this->assertSame(0, $exit, $out);
        $this->assertCount(1, $calls);
        $this->assertSame('/api/applications/shop/push/sync', $calls[0][1]);
        $this->assertSame([['pattern' => 'shop:board', 'public' => true]], $calls[0][2]['json']['rules']);
        $this->assertSame(['k1'], array_column($calls[0][2]['json']['keys']['keys'], 'kid'));
        $this->assertStringContainsString('Push (rules + keys)', $out);
    }

    public function test_nothing_configured_leaves_the_gateway_alone(): void
    {
        $this->app->make(DefinitionRepository::class)->scanAndCache([]);
        $client = $this->createMock(FlowCatalystClient::class);
        $client->expects($this->never())->method('request');
        $this->app->instance(FlowCatalystClient::class, $client);

        $exit = Artisan::call('flowcatalyst:sync');

        $this->assertSame(0, $exit);
        $this->assertStringContainsString('No definitions to sync', Artisan::output());
    }

    public function test_an_unusable_ticket_key_is_reported_and_nothing_is_sent(): void
    {
        $this->app->make(DefinitionRepository::class)->scanAndCache([]);
        $this->app['config']->set('flowcatalyst.push.ticket_key', 'not-a-key');
        $client = $this->createMock(FlowCatalystClient::class);
        $client->expects($this->never())->method('request');
        $this->app->instance(FlowCatalystClient::class, $client);

        $exit = Artisan::call('flowcatalyst:sync');

        $this->assertSame(1, $exit);
        $this->assertStringContainsString('ticket key', Artisan::output());
    }

    public function test_dry_run_lists_the_push_declaration_and_sends_nothing(): void
    {
        $this->app->make(DefinitionRepository::class)->scanAndCache([]);
        $this->app['config']->set('flowcatalyst.push.rules', [['pattern' => 'shop:orders:{id}', 'requires' => ['shop:orders:order:view']]]);
        $this->app['config']->set('flowcatalyst.push.ticket_key', 'k1:' . self::SEED);
        $client = $this->createMock(FlowCatalystClient::class);
        $client->expects($this->never())->method('request');
        $this->app->instance(FlowCatalystClient::class, $client);

        $exit = Artisan::call('flowcatalyst:sync', ['--dry-run' => true]);

        $this->assertSame(0, $exit);
        $out = Artisan::output();
        $this->assertStringContainsString('rule shop:orders:{id}', $out);
        $this->assertStringContainsString('ticket key k1', $out);
    }

    public function test_the_only_flags_skip_push(): void
    {
        $this->app->make(DefinitionRepository::class)->scanAndCache([]);
        $this->app['config']->set('flowcatalyst.push.rules', [['pattern' => 'shop:board', 'public' => true]]);
        $client = $this->createMock(FlowCatalystClient::class);
        $client->expects($this->never())->method('request');
        $this->app->instance(FlowCatalystClient::class, $client);

        Artisan::call('flowcatalyst:sync', ['--roles' => true]);
    }
}
