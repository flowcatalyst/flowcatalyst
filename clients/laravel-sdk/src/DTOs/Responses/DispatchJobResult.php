<?php

declare(strict_types=1);

namespace FlowCatalyst\DTOs\Responses;

/**
 * One item's outcome from `POST /api/dispatch-jobs/batch`, in the order the
 * items were sent: `{id, status, error?}`.
 *
 * `id` is the dispatch job's id. For an item whose idempotency key was already
 * held it is the id of the job that holds it (nothing new was created).
 * `status` is the platform's SCREAMING_SNAKE outcome (`SUCCESS`, or a failure
 * such as `BAD_REQUEST`); `error` is set when the platform gave one.
 */
final class DispatchJobResult
{
    public function __construct(
        public readonly string $id,
        public readonly string $status,
        public readonly ?string $error = null,
    ) {}

    /**
     * @param array<string, mixed> $data
     */
    public static function fromArray(array $data): self
    {
        $error = $data['error'] ?? null;

        return new self(
            id: (string) ($data['id'] ?? ''),
            status: (string) ($data['status'] ?? ''),
            error: $error !== null && $error !== '' ? (string) $error : null,
        );
    }

    public function isSuccess(): bool
    {
        return $this->status === 'SUCCESS';
    }
}
