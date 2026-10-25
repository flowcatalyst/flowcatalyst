<?php

declare(strict_types=1);

namespace FlowCatalyst\DTOs\Ingestion;

/**
 * Where an ingestion's data comes from: `{kind: ..., <settings>}`.
 *
 * An sftp source without `knownHosts` (the server's host key as a known_hosts
 * line) is refused by the runtime unless it sets `acceptAnyHostKey: true`,
 * which connects without checking who the server is (whoever answers receives
 * the credential), so turning it on needs the bind-secrets permission.
 *
 * The settings differ by kind and are validated by the platform, so they are
 * carried as an array. Secrets in settings are the secret's NAME
 * (`['password' => ['name' => 'sftp-password']]`), never a value. A kind
 * supplied by a runtime extension (or `camel-route`) is written with
 * {@see custom()}.
 */
final class IngestionSource
{
    public const HTTP = 'http';
    public const SFTP = 'sftp';
    public const S3 = 's3';
    public const SQS = 'sqs';
    public const RABBITMQ = 'rabbitmq';
    public const DATABASE_POLL = 'database-poll';
    public const API_POLL = 'api-poll';
    public const CUSTOM = 'custom';

    /**
     * @param string $kind One of the constants above
     * @param array<string, mixed> $settings The kind's fields, flat beside `kind`
     */
    public function __construct(
        public readonly string $kind,
        public readonly array $settings = [],
    ) {}

    /**
     * A runtime-supplied kind: `{kind: custom, type: <type>, settings: {...}}`.
     *
     * @param array<string, mixed> $settings
     */
    public static function custom(string $type, array $settings = []): self
    {
        $fields = ['type' => $type];
        if ($settings !== []) {
            $fields['settings'] = $settings;
        }

        return new self(self::CUSTOM, $fields);
    }

    /**
     * @param array<string, mixed> $data
     */
    public static function fromArray(array $data): self
    {
        $kind = (string) ($data['kind'] ?? '');
        unset($data['kind']);

        return new self($kind, $data);
    }

    /**
     * @return array<string, mixed>
     */
    public function toArray(): array
    {
        return ['kind' => $this->kind] + $this->settings;
    }
}
