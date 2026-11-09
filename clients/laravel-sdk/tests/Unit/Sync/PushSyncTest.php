<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Unit\Sync;

use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\Push\PushRuleDefinition;
use FlowCatalyst\Push\TicketMinter;
use FlowCatalyst\Sync\DefinitionSynchronizer;
use FlowCatalyst\Sync\SyncDefinitionSet;
use FlowCatalyst\Sync\SyncOptions;
use PHPUnit\Framework\TestCase;

/**
 * The push gateway's rules + ticket keys ride sync as one more step:
 * `POST /api/applications/{appCode}/push/sync` with {rules, keys: JWKS}.
 */
final class PushSyncTest extends TestCase
{
    private const SEED = '5BgJptHbkz4uIeRqgLz3Ws_38Ovwi1uCpSdyW45yMdc';

    private const RESPONSE = [
        'applicationCode' => 'shop',
        'rules' => ['created' => 2, 'updated' => 0, 'deleted' => 0],
        'keys' => ['created' => 1, 'updated' => 0, 'deleted' => 0],
        'syncedPatterns' => ['shop:orders:{id}', 'shop:board'],
        'syncedKids' => ['k1'],
    ];

    /**
     * @param list<array{0: string, 1: string, 2: array}> $calls
     */
    private function synchronizer(array &$calls, array|\Throwable $response = self::RESPONSE): DefinitionSynchronizer
    {
        $client = $this->createMock(FlowCatalystClient::class);
        $client->method('request')->willReturnCallback(function (string $method, string $path, array $options = []) use (&$calls, $response) {
            $calls[] = [$method, $path, $options];
            if ($response instanceof \Throwable) {
                throw $response;
            }
            return $response;
        });
        return new DefinitionSynchronizer($client);
    }

    private function set(): SyncDefinitionSet
    {
        return SyncDefinitionSet::forApplication('shop')->withPush(
            [
                ['pattern' => 'shop:orders:{id}', 'requires' => ['shop:orders:order:view']],
                new PushRuleDefinition('shop:board', public: true),
            ],
            TicketMinter::fromConfig('shop', 'k1:' . self::SEED),
        );
    }

    public function test_sync_posts_rules_and_the_jwks_and_reports_the_gateways_counts(): void
    {
        $calls = [];
        $result = $this->synchronizer($calls)->sync($this->set());

        $this->assertCount(1, $calls);
        [$method, $path, $options] = $calls[0];
        $this->assertSame('POST', $method);
        $this->assertSame('/api/applications/shop/push/sync', $path);
        $this->assertSame([
            'rules' => [
                ['pattern' => 'shop:orders:{id}', 'requires' => ['shop:orders:order:view']],
                ['pattern' => 'shop:board', 'public' => true],
            ],
            'keys' => TicketMinter::fromConfig('shop', 'k1:' . self::SEED)->jwks(),
        ], $options['json']);

        $this->assertSame(3, $result->push['created'], 'rules + keys');
        $this->assertSame(['created' => 2, 'updated' => 0, 'deleted' => 0], $result->push['rules']);
        $this->assertSame(['shop:orders:{id}', 'shop:board'], $result->push['syncedPatterns']);
        $this->assertSame(['k1'], $result->push['syncedKids']);
        $this->assertTrue($result->hasChanges());
        $this->assertFalse($result->hasErrors());
        $this->assertSame(3, $result->getTotals()['created']);
    }

    public function test_a_refusal_lands_in_the_results_push_error(): void
    {
        $calls = [];
        $result = $this->synchronizer($calls, new \RuntimeException('PATTERN_NOT_OWNED'))->sync($this->set());
        $this->assertTrue($result->hasErrors());
        $this->assertSame(['push' => 'PATTERN_NOT_OWNED'], $result->getErrors());
    }

    public function test_a_set_without_push_leaves_the_gateway_alone_and_options_can_skip_it(): void
    {
        $calls = [];
        $this->synchronizer($calls)->sync(SyncDefinitionSet::forApplication('shop'));
        $this->assertSame([], $calls);

        $this->synchronizer($calls)->sync($this->set(), new SyncOptions(syncPush: false));
        $this->assertSame([], $calls);

        // the "only" factories never touch push; pushOnly touches nothing else
        foreach ([SyncOptions::rolesOnly(), SyncOptions::eventTypesOnly(), SyncOptions::subscriptionsOnly(), SyncOptions::connectionsOnly(),
            SyncOptions::dispatchPoolsOnly(), SyncOptions::principalsOnly(), SyncOptions::processesOnly(), SyncOptions::scheduledJobsOnly(),
            SyncOptions::openapiOnly(), SyncOptions::withRemoveUnlisted()->withRemoveUnlistedEnabled()] as $i => $options) {
            $this->assertSame($i === 9, $options->syncPush, "option #{$i}");
        }
        $this->assertTrue(SyncOptions::pushOnly()->syncPush);
        $this->assertFalse(SyncOptions::pushOnly()->syncRoles);

        $this->synchronizer($calls)->sync($this->set(), SyncOptions::pushOnly());
        $this->assertCount(1, $calls);
    }

    public function test_sync_grouped_merges_push_declarations_of_one_application(): void
    {
        $calls = [];
        $a = TicketMinter::fromConfig('shop', 'k1:' . self::SEED);
        $s1 = SyncDefinitionSet::forApplication('shop')->withPush([['pattern' => 'shop:a', 'public' => true]], $a);
        $s2 = SyncDefinitionSet::forApplication('shop')->forClient('acme')->withPush([['pattern' => 'shop:b', 'public' => true]], $a->jwks());

        $this->synchronizer($calls)->syncGrouped([$s1, $s2]);

        $this->assertCount(1, $calls);
        $body = $calls[0][2]['json'];
        $this->assertSame(['shop:a', 'shop:b'], array_column($body['rules'], 'pattern'));
        $this->assertCount(1, $body['keys']['keys'], 'keys de-duplicated by kid');
    }

    public function test_a_set_with_push_is_not_empty(): void
    {
        $this->assertTrue(SyncDefinitionSet::forApplication('shop')->isEmpty());
        $this->assertFalse($this->set()->isEmpty());
    }
}
