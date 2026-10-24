<?php

declare(strict_types=1);

namespace FlowCatalyst\DTOs\Ingestion;

/**
 * The dispatch job each record of an ingestion becomes. `pool`, `connection`
 * and `serviceAccount` are codes, resolved by the platform at save.
 * `connection` and `serviceAccount` are exclusive; neither means unsigned
 * delivery.
 */
final class IngestionTarget
{
    public const MODE_IMMEDIATE = 'IMMEDIATE';
    public const MODE_NEXT_ON_ERROR = 'NEXT_ON_ERROR';
    public const MODE_BLOCK_ON_ERROR = 'BLOCK_ON_ERROR';

    /**
     * @param string $mode One of the MODE_* constants
     */
    public function __construct(
        public readonly string $code,
        public readonly string $targetUrl,
        public readonly string $pool,
        public readonly string $mode = self::MODE_IMMEDIATE,
        public readonly int $batchSize = 1,
        public readonly ?string $connection = null,
        public readonly ?string $serviceAccount = null,
    ) {}

    /**
     * @param array<string, mixed> $data
     */
    public static function fromArray(array $data): self
    {
        return new self(
            code: (string) $data['code'],
            targetUrl: (string) $data['targetUrl'],
            pool: (string) $data['pool'],
            mode: (string) ($data['mode'] ?? self::MODE_IMMEDIATE),
            batchSize: (int) ($data['batchSize'] ?? 1),
            connection: isset($data['connection']) ? (string) $data['connection'] : null,
            serviceAccount: isset($data['serviceAccount']) ? (string) $data['serviceAccount'] : null,
        );
    }

    /**
     * @return array<string, mixed>
     */
    public function toArray(): array
    {
        return array_filter([
            'code' => $this->code,
            'targetUrl' => $this->targetUrl,
            'pool' => $this->pool,
            'connection' => $this->connection,
            'serviceAccount' => $this->serviceAccount,
            'mode' => $this->mode,
            'batchSize' => $this->batchSize,
        ], static fn($v) => $v !== null);
    }
}
