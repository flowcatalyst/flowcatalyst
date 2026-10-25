<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Unit\Client\Resources;

use FlowCatalyst\Client\Auth\UserTokenProvider;
use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\DTOs\Ingestion\IngestionDefinition;
use FlowCatalyst\DTOs\Ingestion\IngestionSource;
use FlowCatalyst\DTOs\Ingestion\IngestionSplit;
use FlowCatalyst\DTOs\Ingestion\IngestionTarget;
use FlowCatalyst\Exceptions\FlowCatalystException;
use FlowCatalyst\Exceptions\ValidationException;
use GuzzleHttp\Client;
use GuzzleHttp\Handler\MockHandler;
use GuzzleHttp\HandlerStack;
use GuzzleHttp\Middleware;
use GuzzleHttp\Psr7\Response;
use PHPUnit\Framework\TestCase;

/**
 * The ingestion resources through the real FlowCatalystClient with a mocked
 * HTTP layer: method, path (with encoding), query only when given, bodies,
 * error statuses and cursor handling.
 */
final class IngestionsTest extends TestCase
{
    private \ArrayObject $history;

    /** @param array<int, Response> $responses */
    private function client(array $responses): FlowCatalystClient
    {
        $this->history = new \ArrayObject();
        $client = new FlowCatalystClient(
            tokenProvider: new UserTokenProvider('tok'),
            baseUrl: 'https://fc.test',
            retryAttempts: 1,
            retryDelay: 1,
        );

        $stack = HandlerStack::create(new MockHandler($responses));
        $stack->push(Middleware::history($this->history));
        $prop = new \ReflectionProperty(FlowCatalystClient::class, 'httpClient');
        $prop->setAccessible(true);
        $prop->setValue($client, new Client([
            'handler' => $stack,
            'http_errors' => false,
            'base_uri' => 'https://fc.test',
        ]));

        return $client;
    }

    private function json(int $status, array $body): Response
    {
        return new Response($status, [], json_encode($body));
    }

    private function sent(int $i = 0): array
    {
        $request = $this->history[$i]['request'];

        return [
            $request->getMethod(),
            $request->getUri()->getPath() . ($request->getUri()->getQuery() !== '' ? '?' . $request->getUri()->getQuery() : ''),
            (string) $request->getBody(),
        ];
    }

    private function definition(): IngestionDefinition
    {
        return new IngestionDefinition(
            name: 'Orders drop',
            source: new IngestionSource(IngestionSource::SFTP, [
                'host' => 'sftp.example.com',
                'port' => 22,
                'directory' => '/out',
                'username' => 'svc',
                'credential' => ['name' => 'sftp-password'],
                'every' => 'PT5M',
            ]),
            split: IngestionSplit::csv(true),
            target: new IngestionTarget(
                code: 'order',
                targetUrl: 'https://app.example.com/hook',
                pool: 'default',
                mode: IngestionTarget::MODE_NEXT_ON_ERROR,
                batchSize: 10,
                connection: 'app-conn',
            ),
            keepFailedPayloads: true,
        );
    }

    private function row(array $extra = []): array
    {
        return $this->definition()->toArray() + $extra + [
            'id' => 'ing_1', 'applicationId' => 'app_1', 'applicationCode' => 'edi',
            'clientId' => 'clt_1', 'clientIdentifier' => 'acme', 'code' => 'orders',
            'status' => 'ACTIVE', 'version' => 3, 'latestVersion' => 4,
            'createdAt' => '2026-10-24T00:00:00Z', 'updatedAt' => '2026-10-24T00:00:00Z',
        ];
    }

    public function test_list_sends_no_query_without_filters_and_pages_by_cursor(): void
    {
        $client = $this->client([
            $this->json(200, ['ingestions' => [$this->row()], 'hasMore' => true, 'nextCursor' => 'c1']),
            $this->json(200, ['ingestions' => [], 'hasMore' => false]),
        ]);

        $page = $client->ingestions()->list();
        $next = $client->ingestions()->list(after: $page->nextCursor, limit: 50);

        $this->assertSame(['GET', '/api/ingestions', ''], $this->sent(0));
        $this->assertSame('/api/ingestions?after=c1&limit=50', $this->sent(1)[1]);
        $this->assertTrue($page->hasMore);
        $this->assertSame('c1', $page->nextCursor);
        $this->assertSame('orders', $page->ingestions[0]->code);
        $this->assertFalse($next->hasMore);
        $this->assertNull($next->nextCursor);
    }

    public function test_list_filters_are_encoded_and_only_sent_when_given(): void
    {
        $client = $this->client([$this->json(200, ['ingestions' => [], 'hasMore' => false])]);

        $client->ingestions()->list(applicationCode: 'edi', clientId: 'acme corp', q: 'a&b', status: 'PAUSED', kind: 'sftp');

        $this->assertSame(
            '/api/ingestions?applicationCode=edi&clientId=acme%20corp&q=a%26b&status=PAUSED&kind=sftp',
            $this->sent()[1],
        );
    }

    public function test_create_posts_the_definition_and_live_only_when_given(): void
    {
        $client = $this->client([$this->json(201, $this->row()), $this->json(201, $this->row())]);

        $created = $client->ingestions()->create('edi', 'acme', 'orders', $this->definition());
        $client->ingestions()->create('edi', 'acme', 'orders', $this->definition(), live: false);

        [$method, $path, $body] = $this->sent(0);
        $decoded = json_decode($body, true);
        $this->assertSame(['POST', '/api/ingestions'], [$method, $path]);
        $this->assertSame('edi', $decoded['application']);
        $this->assertSame('acme', $decoded['client']);
        $this->assertSame('orders', $decoded['code']);
        $this->assertArrayNotHasKey('live', $decoded);
        $this->assertSame('sftp', $decoded['source']['kind']);
        $this->assertSame(['name' => 'sftp-password'], $decoded['source']['credential']);
        $this->assertSame(['kind' => 'csv', 'header' => true], $decoded['split']);
        $this->assertSame('NEXT_ON_ERROR', $decoded['target']['mode']);
        $this->assertSame('app-conn', $decoded['target']['connection']);
        $this->assertArrayNotHasKey('serviceAccount', $decoded['target']);
        $this->assertFalse(json_decode($this->sent(1)[2], true)['live']);
        $this->assertSame(3, $created->version);
        $this->assertSame('acme', $created->clientIdentifier);
    }

    public function test_custom_source_writes_kind_custom_with_settings(): void
    {
        $source = IngestionSource::custom('mix', ['site' => 'depot-1']);

        $this->assertSame(
            ['kind' => 'custom', 'type' => 'mix', 'settings' => ['site' => 'depot-1']],
            $source->toArray(),
        );
        $this->assertSame(['kind' => 'custom', 'type' => 'x'], IngestionSource::custom('x')->toArray());
    }

    public function test_get_encodes_the_id_and_maps_the_response(): void
    {
        $client = $this->client([$this->json(200, $this->row(['status' => 'PAUSED']))]);

        $ingestion = $client->ingestions()->get('ing/1 x');

        $this->assertSame('/api/ingestions/ing%2F1%20x', $this->sent()[1]);
        $this->assertTrue($ingestion->isPaused());
        $this->assertSame('app-conn', $ingestion->definition->target->connection);
    }

    public function test_update_sends_expected_version_in_the_body(): void
    {
        $client = $this->client([$this->json(200, $this->row())]);

        $client->ingestions()->update('ing_1', $this->definition(), 3);

        [$method, $path, $body] = $this->sent();
        $this->assertSame(['PUT', '/api/ingestions/ing_1'], [$method, $path]);
        $this->assertSame(3, json_decode($body, true)['expectedVersion']);
    }

    public function test_delete_pause_resume_use_their_methods_and_paths(): void
    {
        $client = $this->client([
            new Response(204),
            $this->json(200, $this->row(['status' => 'PAUSED'])),
            $this->json(200, $this->row()),
        ]);

        $client->ingestions()->delete('ing_1');
        $paused = $client->ingestions()->pause('ing_1');
        $resumed = $client->ingestions()->resume('ing_1');

        $this->assertSame(['DELETE', '/api/ingestions/ing_1'], array_slice($this->sent(0), 0, 2));
        $this->assertSame(['POST', '/api/ingestions/ing_1/pause'], array_slice($this->sent(1), 0, 2));
        $this->assertSame(['POST', '/api/ingestions/ing_1/resume'], array_slice($this->sent(2), 0, 2));
        $this->assertTrue($paused->isPaused());
        $this->assertFalse($resumed->isPaused());
    }

    public function test_versions_save_and_make_live(): void
    {
        $version = $this->definition()->toArray() + ['id' => 'ver_1', 'version' => 4, 'live' => false, 'createdAt' => 'x'];
        $client = $this->client([
            $this->json(200, ['versions' => [$version]]),
            $this->json(201, $version),
            $this->json(200, $this->row()),
        ]);

        $versions = $client->ingestions()->versions('ing_1');
        $saved = $client->ingestions()->saveVersion('ing_1', $this->definition());
        $client->ingestions()->makeVersionLive('ing_1', 4);

        $this->assertSame(['GET', '/api/ingestions/ing_1/versions'], array_slice($this->sent(0), 0, 2));
        $this->assertSame(4, $versions[0]->version);
        $this->assertSame(['POST', '/api/ingestions/ing_1/versions'], array_slice($this->sent(1), 0, 2));
        $this->assertArrayNotHasKey('live', json_decode($this->sent(1)[2], true));
        $this->assertFalse($saved->live);
        $this->assertSame(['POST', '/api/ingestions/ing_1/versions/4/live'], array_slice($this->sent(2), 0, 2));
    }

    public function test_definition_returns_the_yaml_text(): void
    {
        $yaml = "name: Orders drop\nsource:\n  kind: sftp\n";
        $client = $this->client([new Response(200, ['Content-Type' => 'application/yaml'], $yaml)]);

        $this->assertSame($yaml, $client->ingestions()->definition('ing_1'));
        $this->assertSame('/api/ingestions/ing_1/definition', $this->sent()[1]);
    }

    public function test_definition_round_trips_through_array_form(): void
    {
        $definition = $this->definition();

        $again = IngestionDefinition::fromArray($definition->toArray());

        $this->assertSame($definition->toArray(), $again->toArray());
        $this->assertSame($definition->toArray(), IngestionDefinition::fromArray($this->row())->toArray());
    }

    public function test_history_failures_and_source_kinds(): void
    {
        $client = $this->client([
            $this->json(200, ['entries' => [], 'hasMore' => false]),
            $this->json(200, ['failures' => [], 'hasMore' => true, 'nextCursor' => 'f9']),
            $this->json(200, ['kinds' => [['kind' => 'http', 'builtIn' => true, 'singleOwner' => false]]]),
        ]);

        $client->ingestions()->history('ing_1', limit: 20, offset: 40);
        $failures = $client->ingestions()->failures('ing_1', after: 'f1');
        $kinds = $client->ingestions()->sourceKinds();

        $this->assertSame('/api/ingestions/ing_1/history?limit=20&offset=40', $this->sent(0)[1]);
        $this->assertSame('/api/ingestions/ing_1/failures?after=f1', $this->sent(1)[1]);
        $this->assertSame('f9', $failures['nextCursor']);
        $this->assertSame('/api/ingestion-source-kinds', $this->sent(2)[1]);
        $this->assertSame('http', $kinds[0]['kind']);
    }

    public function test_failures_filters_dismiss_and_secrets(): void
    {
        $failure = [
            'id' => 'igf_1', 'kind' => 'JOBS_REJECTED', 'category' => 'rejected', 'message' => 'refused',
            'afterRecords' => 0, 'occurredAt' => 't', 'createdAt' => 't',
            'dismissedAt' => 't2', 'dismissedBy' => 'prn_1',
        ];
        $client = $this->client([
            $this->json(200, ['failures' => [], 'hasMore' => false]),
            $this->json(200, ['failures' => [], 'hasMore' => false]),
            $this->json(200, $failure),
            $this->json(200, ['secrets' => [[
                'name' => 'sftp-password', 'bound' => true,
                'fetches' => [['runtimeId' => 'rt-1', 'version' => 3, 'lastFetchedAt' => 't', 'fetchCount' => 7]],
            ]]]),
        ]);

        $client->ingestions()->failures('ing_1', category: 'rejected', undismissed: true);
        $client->ingestions()->failures('ing_1', category: 'payload', undismissed: false);
        $dismissed = $client->ingestions()->dismissFailure('ing_1', 'igf/1');
        $secrets = $client->ingestions()->secrets('ing_1');

        $this->assertSame('/api/ingestions/ing_1/failures?category=rejected&undismissed=true', $this->sent(0)[1]);
        $this->assertSame('/api/ingestions/ing_1/failures?category=payload', $this->sent(1)[1]);
        $this->assertSame(['POST', '/api/ingestions/ing_1/failures/igf%2F1/dismiss', ''], $this->sent(2));
        $this->assertSame('prn_1', $dismissed['dismissedBy']);
        $this->assertSame(['GET', '/api/ingestions/ing_1/secrets', ''], $this->sent(3));
        $this->assertSame(7, $secrets[0]['fetches'][0]['fetchCount']);
        $this->assertArrayNotHasKey('value', $secrets[0]);
    }

    public function test_a_create_without_promote_answers_a_draft_with_notices(): void
    {
        $client = $this->client([
            $this->json(201, $this->row([
                'notices' => 2, 'latestNotice' => 'jobs refused', 'latestNoticeAt' => 't',
                'warnings' => ['no host key'],
                'save' => ['outcome' => 'DRAFT', 'version' => 1, 'awaitingPromote' => true, 'message' => 'saved as a draft'],
            ])),
            $this->json(200, ['versions' => [$this->definition()->toArray() + [
                'id' => 'igv_1', 'version' => 1, 'live' => false, 'draft' => true, 'digest' => 'ab12', 'createdAt' => 't',
            ]]]),
            $this->json(200, $this->row()),
        ]);

        $created = $client->ingestions()->create('edi', 'acme', 'orders', $this->definition(), live: true);
        $versions = $client->ingestions()->versions('ing_1');
        $older = $client->ingestions()->get('ing_1');

        $this->assertNotNull($created->save);
        $this->assertSame('DRAFT', $created->save->outcome);
        $this->assertTrue($created->save->awaitingPromote);
        $this->assertTrue($created->save->isDraft());
        $this->assertSame(2, $created->notices);
        $this->assertSame('jobs refused', $created->latestNotice);
        $this->assertSame(['no host key'], $created->warnings);
        $this->assertTrue($versions[0]->draft);
        $this->assertSame('ab12', $versions[0]->digest);
        $this->assertSame(0, $older->notices);
        $this->assertNull($older->save);
    }

    public function test_sftp_accept_any_host_key_is_sent_as_given(): void
    {
        $source = new IngestionSource(IngestionSource::SFTP, [
            'host' => 'h', 'port' => 22, 'directory' => '/in', 'username' => 'u',
            'credential' => ['name' => 'sftp-password'], 'acceptAnyHostKey' => true, 'every' => 'PT5M',
        ]);

        $this->assertTrue($source->toArray()['acceptAnyHostKey']);
        $this->assertSame($source->toArray(), IngestionSource::fromArray($source->toArray())->toArray());
    }

    public function test_connection_test_by_id_and_by_definition(): void
    {
        $client = $this->client([
            $this->json(202, ['id' => 'tst_1', 'kind' => 'sftp', 'status' => 'PENDING']),
            $this->json(202, ['id' => 'tst_2', 'kind' => 'sftp', 'status' => 'PENDING']),
            $this->json(200, ['id' => 'tst_1', 'kind' => 'sftp', 'status' => 'DONE', 'outcome' => 'RESULTS']),
        ]);

        $client->ingestions()->testConnection(ingestionId: 'ing_1');
        $client->ingestions()->testConnection(definition: $this->definition(), application: 'edi', client: 'acme');
        $done = $client->ingestions()->getTest('tst_1');

        $this->assertSame(['POST', '/api/ingestion-tests', '{"ingestionId":"ing_1"}'], $this->sent(0));
        $second = json_decode($this->sent(1)[2], true);
        $this->assertSame('edi', $second['application']);
        $this->assertSame('Orders drop', $second['definition']['name']);
        $this->assertArrayNotHasKey('ingestionId', $second);
        $this->assertSame('/api/ingestion-tests/tst_1', $this->sent(2)[1]);
        $this->assertSame('RESULTS', $done['outcome']);
    }

    public function test_error_statuses(): void
    {
        $client = $this->client([
            $this->json(400, ['error' => 'INGESTION_TARGET_INVALID', 'message' => 'pool not found']),
            $this->json(403, ['error' => 'FORBIDDEN', 'message' => 'no']),
            $this->json(404, ['error' => 'INGESTION_NOT_FOUND', 'message' => 'missing']),
            $this->json(409, ['error' => 'INGESTION_CODE_EXISTS', 'message' => 'exists']),
            $this->json(422, ['error' => 'VALIDATION', 'message' => 'bad body']),
        ]);

        $codes = [];
        foreach ([400, 403, 404, 409] as $status) {
            try {
                $client->ingestions()->create('edi', 'acme', 'orders', $this->definition());
                $this->fail('expected FlowCatalystException');
            } catch (FlowCatalystException $e) {
                $codes[$status] = [$e->getCode(), $e->getContext()['error']];
            }
        }

        $this->assertSame([400, 'INGESTION_TARGET_INVALID'], $codes[400]);
        $this->assertSame([403, 'FORBIDDEN'], $codes[403]);
        $this->assertSame([404, 'INGESTION_NOT_FOUND'], $codes[404]);
        $this->assertSame([409, 'INGESTION_CODE_EXISTS'], $codes[409]);
        $this->assertCount(4, $this->history, 'none of them is retried');

        $this->expectException(ValidationException::class);
        $client->ingestions()->get('ing_1');
    }

    public function test_sets_apply_dry_run_query_and_files(): void
    {
        $plan = ['name' => 's/1', 'dryRun' => true, 'changes' => []];
        $client = $this->client([
            $this->json(200, $plan),
            $this->json(200, ['name' => 's/1', 'dryRun' => false, 'applyId' => 'sap_1', 'changes' => []]),
        ]);
        $item = $this->definition()->toArray() + ['application' => 'edi', 'client' => 'acme', 'code' => 'orders'];

        $client->ingestionSets()->apply('s/1', [$item], dryRun: true);
        $client->ingestionSets()->apply('s/1', [$item], files: [['path' => 'a.yaml', 'content' => 'x']]);

        $this->assertSame('PUT', $this->sent(0)[0]);
        $this->assertSame('/api/ingestion-sets/s%2F1?dryRun=true', $this->sent(0)[1]);
        $this->assertArrayNotHasKey('files', json_decode($this->sent(0)[2], true));
        $this->assertSame('/api/ingestion-sets/s%2F1', $this->sent(1)[1]);
        $this->assertSame('a.yaml', json_decode($this->sent(1)[2], true)['files'][0]['path']);
    }

    public function test_sets_reads_reapply_release_and_owned_conflict(): void
    {
        $client = $this->client([
            $this->json(200, ['sets' => [['id' => 'ist_1', 'name' => 's1']]]),
            $this->json(200, ['applies' => [['id' => 'sap_1']]]),
            $this->json(200, ['id' => 'sap_1']),
            $this->json(200, ['name' => 's1', 'dryRun' => true, 'changes' => []]),
            $this->json(200, ['name' => 's1', 'dryRun' => false, 'changes' => []]),
            $this->json(200, ['name' => 's1', 'released' => 2]),
            $this->json(409, ['error' => 'INGESTION_SET_OWNED', 'message' => 'owned']),
        ]);
        $sets = $client->ingestionSets();

        $this->assertSame('s1', $sets->list()[0]['name']);
        $this->assertSame('sap_1', $sets->applies('s1')[0]['id']);
        $sets->getApply('s1', 'sap_1');
        $sets->reapply('s1', 'sap_1', dryRun: true);
        $sets->reapply('s1', 'sap_1');
        $sets->release('s1');

        $this->assertSame('/api/ingestion-sets', $this->sent(0)[1]);
        $this->assertSame('/api/ingestion-sets/s1/applies', $this->sent(1)[1]);
        $this->assertSame('/api/ingestion-sets/s1/applies/sap_1', $this->sent(2)[1]);
        $this->assertSame(['POST', '/api/ingestion-sets/s1/applies/sap_1/reapply?dryRun=true'], array_slice($this->sent(3), 0, 2));
        $this->assertSame('/api/ingestion-sets/s1/applies/sap_1/reapply', $this->sent(4)[1]);
        $this->assertSame(['POST', '/api/ingestion-sets/s1/release'], array_slice($this->sent(5), 0, 2));

        try {
            $sets->apply('s1', []);
            $this->fail('expected FlowCatalystException');
        } catch (FlowCatalystException $e) {
            $this->assertSame(409, $e->getCode());
            $this->assertSame('INGESTION_SET_OWNED', $e->getContext()['error']);
        }
    }
}
