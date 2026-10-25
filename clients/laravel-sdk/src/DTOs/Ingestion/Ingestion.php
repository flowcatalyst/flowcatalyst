<?php

declare(strict_types=1);

namespace FlowCatalyst\DTOs\Ingestion;

/**
 * An ingestion as the platform returns it. `version` is the live version
 * (0 when none is live); pass it back as `expectedVersion` on update.
 */
final class Ingestion
{
    public function __construct(
        public readonly string $id,
        public readonly string $applicationId,
        public readonly string $applicationCode,
        public readonly string $clientId,
        public readonly string $clientIdentifier,
        public readonly string $code,
        public readonly string $status,
        public readonly int $version,
        public readonly int $latestVersion,
        public readonly IngestionDefinition $definition,
        public readonly string $createdAt = '',
        public readonly string $updatedAt = '',
        public readonly ?string $managedBy = null,
        public readonly ?string $runtimeNotice = null,
        /** Undismissed notices: failures the platform refused (category `rejected`). */
        public readonly int $notices = 0,
        /** The newest undismissed notice's message. */
        public readonly ?string $latestNotice = null,
        public readonly ?string $latestNoticeAt = null,
        /** @var string[] What the definition says that will not run as meant. */
        public readonly array $warnings = [],
        /** What this create or update did (set on those answers only). */
        public readonly ?IngestionSaveOutcome $save = null,
    ) {}

    /**
     * @param array<string, mixed> $data
     */
    public static function fromArray(array $data): self
    {
        return new self(
            id: (string) $data['id'],
            applicationId: (string) $data['applicationId'],
            applicationCode: (string) $data['applicationCode'],
            clientId: (string) $data['clientId'],
            clientIdentifier: (string) $data['clientIdentifier'],
            code: (string) $data['code'],
            status: (string) $data['status'],
            version: (int) $data['version'],
            latestVersion: (int) $data['latestVersion'],
            definition: IngestionDefinition::fromArray($data),
            createdAt: (string) ($data['createdAt'] ?? ''),
            updatedAt: (string) ($data['updatedAt'] ?? ''),
            managedBy: isset($data['managedBy']) ? (string) $data['managedBy'] : null,
            runtimeNotice: isset($data['runtimeNotice']) ? (string) $data['runtimeNotice'] : null,
            notices: (int) ($data['notices'] ?? 0),
            latestNotice: isset($data['latestNotice']) ? (string) $data['latestNotice'] : null,
            latestNoticeAt: isset($data['latestNoticeAt']) ? (string) $data['latestNoticeAt'] : null,
            warnings: array_map('strval', (array) ($data['warnings'] ?? [])),
            save: IngestionSaveOutcome::fromArray(isset($data['save']) && is_array($data['save']) ? $data['save'] : null),
        );
    }

    public function isPaused(): bool
    {
        return $this->status === 'PAUSED';
    }
}
