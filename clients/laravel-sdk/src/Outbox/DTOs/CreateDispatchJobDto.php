<?php

declare(strict_types=1);

namespace FlowCatalyst\Outbox\DTOs;

use FlowCatalyst\Outbox\QualifiedCode;

/**
 * A dispatch job to create: written to the outbox (`Outbox::createDispatchJob`)
 * or sent straight to the platform (`FlowCatalyst::dispatchJobs()->create()` /
 * `->batch()`). Both paths send the same item, built by {@see toBatchItem()},
 * in the shape of `POST /api/dispatch-jobs/batch`.
 *
 * Fields the platform does not honour on that route are kept for source
 * compatibility but are NOT sent: `headers`, `retryStrategy`, `scheduledFor`
 * and `expiresAt` (and their withers). Setting them has no effect.
 */
class CreateDispatchJobDto
{
    /**
     * @param array<string, scalar|null>|list<array{key: string, value: string}> $metadata
     *        Additional metadata. An associative array is sent as the platform's
     *        list of `{key, value}` (key-sorted, values as strings); a list of
     *        `{key, value}` pairs is sent as given.
     * @param array<string, string> $headers Not sent: the platform does not take per-job headers.
     * @param string|null $retryStrategy Not sent: the batch route does not take one.
     * @param \DateTimeInterface|null $scheduledFor Not sent: the batch route does not take one.
     * @param \DateTimeInterface|null $expiresAt Not sent: the batch route does not take one.
     * @param string|null $idempotencyKey Makes the create idempotent: the first job with a key, within
     *        its application (the code's first segment) and client, wins it; a repeat answers that
     *        job's id and creates nothing. At most 100 characters.
     * @param string|null $connectionId The connection the job is delivered through: pausing it holds
     *        the job, and (with no subscription) its service account signs the delivery. Must be global
     *        or the job's client's, shared or the job's application's.
     * @param string|null $clientCode The FlowCatalyst client the job belongs to, by identifier. Ignored
     *        by the platform when $clientId is set. The delivery then carries X-FlowCatalyst-Client.
     * @param string|null $clientId The FlowCatalyst client the job belongs to, by id. Wins over $clientCode.
     */
    public function __construct(
        public readonly string $source,
        public readonly string $code,
        public readonly string $targetUrl,
        public readonly string $payload,
        public readonly string $dispatchPoolId,
        public readonly ?string $subject = null,
        public readonly ?string $correlationId = null,
        public readonly ?string $eventId = null,
        public readonly array $metadata = [],
        public readonly array $headers = [],
        public readonly string $payloadContentType = 'application/json',
        public readonly bool $dataOnly = true,
        public readonly ?string $messageGroup = null,
        public readonly ?string $mode = null,
        public readonly ?int $sequence = null,
        public readonly int $timeoutSeconds = 30,
        public readonly int $maxRetries = 5,
        public readonly ?string $retryStrategy = null,
        public readonly ?\DateTimeInterface $scheduledFor = null,
        public readonly ?\DateTimeInterface $expiresAt = null,
        public readonly ?string $idempotencyKey = null,
        public readonly ?string $externalId = null,
        public readonly ?string $connectionId = null,
        public readonly ?string $queue = null,
        public readonly ?string $clientCode = null,
        public readonly ?string $clientId = null,
    ) {
        QualifiedCode::assert($this->code, 'Dispatch job code');
    }

    /**
     * Create a new dispatch job DTO.
     *
     * @param array|string $payload The payload (will be JSON encoded if array)
     */
    public static function create(
        string $source,
        string $code,
        string $targetUrl,
        array|string $payload,
        string $dispatchPoolId,
    ): self {
        return new self(
            source: $source,
            code: $code,
            targetUrl: $targetUrl,
            payload: is_array($payload) ? json_encode($payload) : $payload,
            dispatchPoolId: $dispatchPoolId,
        );
    }

    /**
     * A copy with the given fields replaced. Every wither goes through here,
     * so none can forget a field.
     *
     * @param array<string, mixed> $changes constructor argument name => value
     */
    private function with(array $changes): self
    {
        return new self(...array_merge(get_object_vars($this), $changes));
    }

    /**
     * Add a correlation ID.
     */
    public function withCorrelationId(string $correlationId): self
    {
        return $this->with(['correlationId' => $correlationId]);
    }

    /**
     * Add a subject.
     */
    public function withSubject(string $subject): self
    {
        return $this->with(['subject' => $subject]);
    }

    /**
     * Add HTTP headers for the webhook.
     *
     * @deprecated Not sent: the platform does not take per-job headers, so these never reached the
     *             delivery. Kept so existing callers keep compiling.
     */
    public function withHeaders(array $headers): self
    {
        return $this->with(['headers' => array_merge($this->headers, $headers)]);
    }

    /**
     * Add metadata (merged with any already set). An associative array is
     * sent as the platform's `{key, value}` list.
     *
     * @param array<string, scalar|null>|list<array{key: string, value: string}> $metadata
     */
    public function withMetadata(array $metadata): self
    {
        return $this->with(['metadata' => array_merge($this->metadata, $metadata)]);
    }

    /**
     * Set the message group for ordered dispatch.
     */
    public function withMessageGroup(string $messageGroup): self
    {
        return $this->with(['messageGroup' => $messageGroup]);
    }

    /**
     * Set the dispatch mode: IMMEDIATE, NEXT_ON_ERROR or BLOCK_ON_ERROR.
     * Controls ordering within the message group; unset defaults to NEXT_ON_ERROR (in-sequence, moving on past a failure).
     */
    public function withMode(string $mode): self
    {
        return $this->with(['mode' => $mode]);
    }

    /**
     * Schedule the job for later execution.
     *
     * @deprecated Not sent: the batch route has no scheduledFor, so the job was always created
     *             for immediate dispatch. Kept so existing callers keep compiling.
     */
    public function scheduledFor(\DateTimeInterface $scheduledFor): self
    {
        return $this->with(['scheduledFor' => $scheduledFor]);
    }

    /**
     * Set an expiration time.
     *
     * @deprecated Not sent: the batch route has no expiresAt. Kept so existing callers keep compiling.
     */
    public function expiresAt(\DateTimeInterface $expiresAt): self
    {
        return $this->with(['expiresAt' => $expiresAt]);
    }

    /**
     * Set an idempotency key (at most 100 characters). The first job with the
     * key, within its application and client, wins it; a repeat answers that
     * job's id and creates nothing. Use one whenever a create may be retried.
     */
    public function withIdempotencyKey(string $idempotencyKey): self
    {
        return $this->with(['idempotencyKey' => $idempotencyKey]);
    }

    /**
     * Set the connection the job is delivered through. With no subscription,
     * the connection's service account signs the delivery.
     */
    public function withConnectionId(string $connectionId): self
    {
        return $this->with(['connectionId' => $connectionId]);
    }

    /**
     * Set the FlowCatalyst client the job belongs to, by identifier (resolved
     * by the platform; ignored when a client id is also set).
     */
    public function withClientCode(string $clientCode): self
    {
        return $this->with(['clientCode' => $clientCode]);
    }

    /**
     * Set the FlowCatalyst client the job belongs to, by id. Wins over a
     * client code.
     */
    public function withClientId(string $clientId): self
    {
        return $this->with(['clientId' => $clientId]);
    }

    /**
     * Set the job's own dispatch priority: DEFAULT or HIGH_PRIORITY, matched
     * ignoring case. Unset stays absent — never silently defaulted — so "not
     * asked for" stays distinguishable from an explicit DEFAULT. Wins over
     * the target subscription's own priority at publish time when set.
     *
     * Not validated here: the platform rejects an invalid value, and
     * duplicating that check client-side would just be another place to drift.
     */
    public function withQueue(string $queue): self
    {
        return $this->with(['queue' => $queue]);
    }

    /**
     * The item as `POST /api/dispatch-jobs/batch` takes it. The one mapping:
     * the outbox row's payload and the direct API call are both this.
     *
     * @return array<string, mixed>
     */
    public function toBatchItem(): array
    {
        $metadata = self::metadataList($this->metadata);

        return array_filter([
            'source' => $this->source,
            'code' => $this->code,
            'targetUrl' => $this->targetUrl,
            'payload' => $this->payload,
            'payloadContentType' => $this->payloadContentType,
            'dispatchPoolId' => $this->dispatchPoolId !== '' ? $this->dispatchPoolId : null,
            'subject' => $this->subject,
            'correlationId' => $this->correlationId,
            'eventId' => $this->eventId,
            'metadata' => $metadata !== [] ? $metadata : null,
            'dataOnly' => $this->dataOnly,
            'messageGroup' => $this->messageGroup,
            'mode' => $this->mode,
            'sequence' => $this->sequence,
            'timeoutSeconds' => $this->timeoutSeconds,
            'maxRetries' => $this->maxRetries,
            'idempotencyKey' => $this->idempotencyKey,
            'externalId' => $this->externalId,
            'connectionId' => $this->connectionId,
            'queue' => $this->queue,
            'clientCode' => $this->clientCode,
            'clientId' => $this->clientId,
        ], fn($v) => $v !== null);
    }

    /**
     * Build the dispatch job payload for the outbox: the batch item
     * ({@see toBatchItem()}), forwarded verbatim by the outbox processor.
     *
     * @return array<string, mixed>
     */
    public function toPayload(): array
    {
        return $this->toBatchItem();
    }

    /**
     * Metadata in the platform's list-of-`{key, value}` shape. An associative
     * array becomes a key-sorted list with string values (a non-string scalar
     * is its JSON text, `null` is `''`); a list of `{key, value}` pairs is
     * kept as given.
     *
     * @param array<array-key, mixed> $metadata
     * @return list<array{key: string, value: string}>
     */
    public static function metadataList(array $metadata): array
    {
        if ($metadata === []) {
            return [];
        }
        if (array_is_list($metadata)) {
            return array_values(array_map(
                static fn($e) => ['key' => (string) ($e['key'] ?? ''), 'value' => self::metadataValue($e['value'] ?? null)],
                $metadata,
            ));
        }
        ksort($metadata, SORT_STRING);
        $out = [];
        foreach ($metadata as $key => $value) {
            $out[] = ['key' => (string) $key, 'value' => self::metadataValue($value)];
        }

        return $out;
    }

    private static function metadataValue(mixed $value): string
    {
        return match (true) {
            $value === null => '',
            is_string($value) => $value,
            default => (string) json_encode($value, JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE),
        };
    }
}
