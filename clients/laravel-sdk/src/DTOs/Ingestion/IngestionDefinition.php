<?php

declare(strict_types=1);

namespace FlowCatalyst\DTOs\Ingestion;

/**
 * The part of an ingestion that is versioned: source, split and target, with
 * the name and the keep-failed-payloads flag. The request bodies of
 * create, update and save-version are this plus their own fields.
 */
final class IngestionDefinition
{
    public function __construct(
        public readonly string $name,
        public readonly IngestionSource $source,
        public readonly IngestionSplit $split,
        public readonly IngestionTarget $target,
        public readonly bool $keepFailedPayloads = false,
        public readonly ?string $note = null,
    ) {}

    /**
     * @param array<string, mixed> $data
     */
    public static function fromArray(array $data): self
    {
        return new self(
            name: (string) $data['name'],
            source: IngestionSource::fromArray($data['source'] ?? []),
            split: IngestionSplit::fromArray($data['split'] ?? []),
            target: IngestionTarget::fromArray($data['target'] ?? []),
            keepFailedPayloads: (bool) ($data['keepFailedPayloads'] ?? false),
            note: isset($data['note']) ? (string) $data['note'] : null,
        );
    }

    /**
     * @return array<string, mixed>
     */
    public function toArray(): array
    {
        return array_filter([
            'name' => $this->name,
            'source' => $this->source->toArray(),
            'split' => $this->split->toArray(),
            'target' => $this->target->toArray(),
            'keepFailedPayloads' => $this->keepFailedPayloads,
            'note' => $this->note,
        ], static fn($v) => $v !== null);
    }
}
