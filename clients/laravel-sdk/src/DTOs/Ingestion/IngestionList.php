<?php

declare(strict_types=1);

namespace FlowCatalyst\DTOs\Ingestion;

/**
 * A page of ingestions from GET /api/ingestions. Pass `nextCursor` as `after`
 * to read the next page.
 */
final class IngestionList
{
    /**
     * @param Ingestion[] $ingestions
     */
    public function __construct(
        public readonly array $ingestions,
        public readonly bool $hasMore,
        public readonly ?string $nextCursor = null,
    ) {}

    /**
     * @param array<string, mixed> $data
     */
    public static function fromArray(array $data): self
    {
        /** @var array<int, array<string, mixed>> $rows */
        $rows = $data['ingestions'] ?? [];

        return new self(
            ingestions: array_map(fn(array $row) => Ingestion::fromArray($row), $rows),
            hasMore: (bool) ($data['hasMore'] ?? false),
            nextCursor: isset($data['nextCursor']) ? (string) $data['nextCursor'] : null,
        );
    }
}
