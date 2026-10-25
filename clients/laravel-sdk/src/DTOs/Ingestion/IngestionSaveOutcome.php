<?php

declare(strict_types=1);

namespace FlowCatalyst\DTOs\Ingestion;

/**
 * What a create, update or version save did (`save` on those answers).
 *
 * `UNCHANGED`: the live version already says it, nothing written. `SAVED`:
 * saved as a new version and made live. `MADE_LIVE`: a saved version that was
 * not live already says it and was made live. `DRAFT`: saved as a new version
 * that is not live (no promote permission, or not asked to be live).
 * `DRAFT_EXISTS`: a saved draft already says it, nothing saved. `version` is
 * the live version or the draft; `awaitingPromote` is true while someone who
 * holds promote must make it live.
 */
final class IngestionSaveOutcome
{
    public const UNCHANGED = 'UNCHANGED';
    public const SAVED = 'SAVED';
    public const MADE_LIVE = 'MADE_LIVE';
    public const DRAFT = 'DRAFT';
    public const DRAFT_EXISTS = 'DRAFT_EXISTS';

    public function __construct(
        public readonly string $outcome,
        public readonly int $version,
        public readonly bool $awaitingPromote,
        public readonly string $message = '',
    ) {}

    /**
     * @param array<string, mixed>|null $data
     */
    public static function fromArray(?array $data): ?self
    {
        if ($data === null) {
            return null;
        }

        return new self(
            outcome: (string) ($data['outcome'] ?? ''),
            version: (int) ($data['version'] ?? 0),
            awaitingPromote: (bool) ($data['awaitingPromote'] ?? false),
            message: (string) ($data['message'] ?? ''),
        );
    }

    public function isDraft(): bool
    {
        return $this->outcome === self::DRAFT || $this->outcome === self::DRAFT_EXISTS;
    }
}
