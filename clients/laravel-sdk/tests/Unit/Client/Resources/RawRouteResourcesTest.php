<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Unit\Client\Resources;

use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\Client\Resources\Clients;
use FlowCatalyst\Client\Resources\EmailDomainMappings;
use FlowCatalyst\Client\Resources\PlatformConfig;
use FlowCatalyst\Client\Resources\Principals;
use FlowCatalyst\Client\Resources\Roles;
use FlowCatalyst\DTOs\Requests\AddSchemaVersionRequest;
use FlowCatalyst\DTOs\Requests\SyncRoleEntry;
use FlowCatalyst\Exceptions\FlowCatalystException;
use FlowCatalyst\Exceptions\ValidationException;
use FlowCatalyst\Sync\EventTypeDefinition;
use PHPUnit\Framework\TestCase;

/**
 * Request building for the resources that replace the apps' raw `request()`
 * workarounds, plus the EventTypeDefinition code payload and the atomic role
 * sync error message. Requests are captured at FlowCatalystClient::request().
 */
final class RawRouteResourcesTest extends TestCase
{
    /** @var array<int, array{0: string, 1: string, 2: array}> */
    private array $calls = [];

    /** @param array<string, mixed> $answer */
    private function client(array $answer = []): FlowCatalystClient
    {
        $this->calls = [];
        $client = $this->createMock(FlowCatalystClient::class);
        $client->method('request')->willReturnCallback(
            function (string $method, string $endpoint, array $options = []) use ($answer) {
                $this->calls[] = [$method, $endpoint, $options];
                return $answer;
            }
        );

        return $client;
    }

    private function configRow(): array
    {
        return [
            'id' => 'cfg1', 'applicationCode' => 'rfp', 'section' => 'mail', 'property' => 'from',
            'scope' => 'GLOBAL', 'valueType' => 'PLAIN', 'value' => 'a@b.c',
            'createdAt' => '2026-01-01T00:00:00Z', 'updatedAt' => '2026-01-02T00:00:00Z',
        ];
    }

    public function test_platform_config_get_builds_path_and_query(): void
    {
        $cfg = new PlatformConfig($this->client($this->configRow()));

        $row = $cfg->get('rfp', 'mail', 'from');
        $cfg->get('rfp', 'mail', 'from', 'clt_1');

        $this->assertSame('a@b.c', $row->value);
        $this->assertSame(['GET', '/api/config/rfp/mail/from', []], $this->calls[0]);
        $this->assertSame('/api/config/rfp/mail/from?clientId=clt_1', $this->calls[1][1]);
    }

    public function test_platform_config_encodes_path_segments(): void
    {
        $cfg = new PlatformConfig($this->client($this->configRow()));
        $cfg->getRaw('a b', 's/x', 'p?q');

        $this->assertSame('/api/config/a%20b/s%2Fx/p%3Fq', $this->calls[0][1]);
    }

    public function test_platform_config_set_sends_value_type_and_optional_fields(): void
    {
        $cfg = new PlatformConfig($this->client($this->configRow()));

        $cfg->set('rfp', 'mail', 'from', 'v');
        $cfg->set('rfp', 'mail', 'from', 'v', PlatformConfig::VALUE_TYPE_SECRET, 'desc', 'clt_1');

        $this->assertSame('PUT', $this->calls[0][0]);
        $this->assertSame('/api/config/rfp/mail/from', $this->calls[0][1]);
        $this->assertSame(['json' => ['value' => 'v', 'valueType' => 'PLAIN']], $this->calls[0][2]);
        $this->assertSame('/api/config/rfp/mail/from?clientId=clt_1', $this->calls[1][1]);
        $this->assertSame(
            ['json' => ['value' => 'v', 'valueType' => 'SECRET', 'description' => 'desc']],
            $this->calls[1][2],
        );
    }

    public function test_platform_config_writes_send_on_behalf_of_only_when_given(): void
    {
        $cfg = new PlatformConfig($this->client($this->configRow()));

        $cfg->set('rfp', 'mail', 'from', 'v', onBehalfOf: 'usr:alice@example.com');
        $cfg->delete('rfp', 'mail', 'from', 'clt_1', 'bob');
        $cfg->delete('rfp', 'mail', 'from', onBehalfOf: '  ');

        $this->assertSame(
            ['json' => ['value' => 'v', 'valueType' => 'PLAIN'], 'headers' => ['X-On-Behalf-Of' => 'usr:alice@example.com']],
            $this->calls[0][2],
        );
        $this->assertSame(
            ['DELETE', '/api/config/rfp/mail/from?clientId=clt_1', ['headers' => [PlatformConfig::ON_BEHALF_OF_HEADER => 'bob']]],
            $this->calls[1],
        );
        $this->assertSame(['DELETE', '/api/config/rfp/mail/from', []], $this->calls[2]);
    }

    public function test_platform_config_rows_carry_the_client_identifier(): void
    {
        $cfg = new PlatformConfig($this->client(['items' => [
            $this->configRow() + ['clientId' => 'clt_1', 'clientIdentifier' => 'acme'],
        ]]));

        $row = $cfg->list('rfp')[0];

        $this->assertSame('clt_1', $row->clientId);
        $this->assertSame('acme', $row->clientIdentifier);
        $this->assertSame('acme', $row->toArray()['clientIdentifier']);
    }

    public function test_platform_config_delete_and_list(): void
    {
        $cfg = new PlatformConfig($this->client(['items' => [$this->configRow()]]));

        $cfg->delete('rfp', 'mail', 'from');
        $items = $cfg->list('rfp');
        $raw = $cfg->listRaw('rfp');

        $this->assertSame(['DELETE', '/api/config/rfp/mail/from', []], $this->calls[0]);
        $this->assertSame(['GET', '/api/platform-config/rfp', []], $this->calls[1]);
        $this->assertCount(1, $items);
        $this->assertSame('from', $items[0]->property);
        $this->assertSame(['items' => [$this->configRow()]], $raw);
    }

    public function test_email_domain_mapping_by_domain(): void
    {
        $mapping = [
            'id' => 'edm1', 'emailDomain' => 'example.com', 'identityProviderId' => 'idp1',
            'scopeType' => 'CLIENT', 'primaryClientId' => 'clt_1',
            'additionalClientIds' => [], 'grantedClientIds' => ['clt_2'],
            'require2fa' => true, 'allowed2faMethods' => ['TOTP'],
            'rememberDeviceEnabled' => false, 'rememberDeviceDays' => 0,
            'createdAt' => 'c', 'updatedAt' => 'u',
        ];
        $res = new EmailDomainMappings($this->client($mapping));

        $dto = $res->getByDomain('example.com');
        $raw = $res->getByDomainRaw('ex ample.com');

        $this->assertSame(['GET', '/api/email-domain-mappings/by-domain/example.com', []], $this->calls[0]);
        $this->assertSame('/api/email-domain-mappings/by-domain/ex%20ample.com', $this->calls[1][1]);
        $this->assertSame('CLIENT', $dto->scopeType);
        $this->assertSame('clt_1', $dto->primaryClientId);
        $this->assertTrue($dto->require2fa);
        $this->assertSame($mapping, $raw);
    }

    public function test_principal_set_client_association(): void
    {
        $principal = [
            'id' => 'prn_1', 'type' => 'USER', 'scope' => 'PARTNER', 'name' => 'N', 'active' => true,
            'roles' => [], 'isAnchorUser' => false, 'grantedClientIds' => ['clt_2'],
        ];
        $res = new Principals($this->client($principal));

        $p = $res->setClientAssociation('prn_1', 'clt_2', 'TO_PARTNER');
        $res->setClientAssociation('prn_1', '*');

        $this->assertSame('PARTNER', $p->scope);
        $this->assertSame(
            ['PUT', '/api/principals/prn_1/client-association', ['json' => ['clientId' => 'clt_2', 'mode' => 'TO_PARTNER']]],
            $this->calls[0],
        );
        $this->assertSame(['json' => ['clientId' => '*']], $this->calls[1][2]);
    }

    public function test_clients_raw_lookups_do_not_default_status(): void
    {
        $answer = ['id' => 'clt_1', 'name' => 'Acme', 'identifier' => 'acme'];
        $res = new Clients($this->client($answer));

        $byId = $res->getRaw('clt_1');
        $bySlug = $res->getByIdentifierRaw('ac me');

        $this->assertSame($answer, $byId);
        $this->assertArrayNotHasKey('status', $byId);
        $this->assertSame($answer, $bySlug);
        $this->assertSame(['GET', '/api/clients/clt_1', []], $this->calls[0]);
        $this->assertSame('/api/clients/by-identifier/ac%20me', $this->calls[1][1]);
        // The typed lookup keeps its documented default.
        $this->assertSame('ACTIVE', $res->get('clt_1')->status);
    }

    public function test_event_type_definition_to_array_carries_code(): void
    {
        $def = new EventTypeDefinition('Created', 'orders', 'fulfil', 'order', 'created', 'd', true);
        $row = $def->toArray();

        $this->assertSame('orders:fulfil:order:created', $row['code']);
        $this->assertSame('orders', $row['application']);
        $this->assertTrue($row['clientScoped']);
        // Round trip stays lossless for existing fromArray callers.
        $this->assertEquals($def, EventTypeDefinition::fromArray($row));
    }

    public function test_add_schema_version_always_sends_version(): void
    {
        $this->assertSame(
            ['version' => '1.0', 'schema' => ['type' => 'object']],
            (new AddSchemaVersionRequest(['type' => 'object']))->toArray(),
        );
        $this->assertSame('2.1', (new AddSchemaVersionRequest([], '2.1'))->toArray()['version']);
    }

    private function failingRolesClient(\Throwable $e): FlowCatalystClient
    {
        $client = $this->createMock(FlowCatalystClient::class);
        $client->method('request')->willThrowException($e);

        return $client;
    }

    public function test_role_sync_failure_names_role_and_reason(): void
    {
        $platform = new FlowCatalystException(
            'sync save failed at index 1',
            500,
            null,
            ['error' => 'PERSIST_BATCH', 'message' => 'sync save failed at index 1', 'details' => ['field' => 'name']],
        );
        $roles = [new SyncRoleEntry('orders:viewer'), new SyncRoleEntry('orders:admin')];

        try {
            (new Roles($this->failingRolesClient($platform)))->sync('orders', $roles);
            $this->fail('expected exception');
        } catch (FlowCatalystException $e) {
            $this->assertSame(FlowCatalystException::class, get_class($e));
            $this->assertSame(500, $e->getCode());
            $this->assertStringStartsWith('sync save failed at index 1', $e->getMessage());
            $this->assertStringContainsString("role 'orders:admin' (index 1)", $e->getMessage());
            $this->assertStringContainsString('code PERSIST_BATCH', $e->getMessage());
            $this->assertStringContainsString('{"field":"name"}', $e->getMessage());
            $this->assertSame('orders:admin', $e->getContext()['failedRole']);
            $this->assertSame(1, $e->getContext()['failedIndex']);
            $this->assertSame('PERSIST_BATCH', $e->getContext()['error']);
            $this->assertSame($platform, $e->getPrevious());
        }
    }

    public function test_role_sync_failure_subclasses_pass_through_unchanged(): void
    {
        $validation = new ValidationException('bad', ['x']);

        try {
            (new Roles($this->failingRolesClient($validation)))->sync('orders', [new SyncRoleEntry('r')]);
            $this->fail('expected exception');
        } catch (\Throwable $e) {
            $this->assertSame($validation, $e);
        }
    }

    public function test_role_sync_failure_without_index_is_untouched(): void
    {
        $platform = new FlowCatalystException('Access forbidden', 403);

        try {
            (new Roles($this->failingRolesClient($platform)))->sync('orders', [new SyncRoleEntry('r')]);
            $this->fail('expected exception');
        } catch (\Throwable $e) {
            $this->assertSame($platform, $e);
        }
    }
}
