<?php

declare(strict_types=1);

namespace FlowCatalyst\Outbox\DTOs;

/**
 * DTO for creating a dispatch job in the outbox.
 */
class CreateDispatchJobDto
{
    /**
     * @param array<string, string> $metadata Additional metadata
     * @param array<string, string> $headers HTTP headers for the webhook
     */
    public function __construct(
        public readonly string $source,
        public readonly string $code,
        public readonly string $targetUrl,
        public readonly string $payload,
        public readonly string $dispatchPoolId,
        public readonly string $partitionId,
        public readonly ?string $subject = null,
        public readonly ?string $correlationId = null,
        public readonly ?string $eventId = null,
        public readonly array $metadata = [],
        public readonly array $headers = [],
        public readonly string $payloadContentType = 'application/json',
        public readonly bool $dataOnly = true,
        public readonly ?string $messageGroup = null,
        public readonly ?int $sequence = null,
        public readonly int $timeoutSeconds = 30,
        public readonly int $maxRetries = 5,
        public readonly ?string $retryStrategy = null,
        public readonly ?\DateTimeInterface $scheduledFor = null,
        public readonly ?\DateTimeInterface $expiresAt = null,
        public readonly ?string $idempotencyKey = null,
        public readonly ?string $externalId = null,
    ) {}

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
        string $partitionId
    ): self {
        return new self(
            source: $source,
            code: $code,
            targetUrl: $targetUrl,
            payload: is_array($payload) ? json_encode($payload) : $payload,
            dispatchPoolId: $dispatchPoolId,
            partitionId: $partitionId,
        );
    }

    /**
     * Add a correlation ID.
     */
    public function withCorrelationId(string $correlationId): self
    {
        return new self(
            source: $this->source,
            code: $this->code,
            targetUrl: $this->targetUrl,
            payload: $this->payload,
            dispatchPoolId: $this->dispatchPoolId,
            partitionId: $this->partitionId,
            subject: $this->subject,
            correlationId: $correlationId,
            eventId: $this->eventId,
            metadata: $this->metadata,
            headers: $this->headers,
            payloadContentType: $this->payloadContentType,
            dataOnly: $this->dataOnly,
            messageGroup: $this->messageGroup,
            sequence: $this->sequence,
            timeoutSeconds: $this->timeoutSeconds,
            maxRetries: $this->maxRetries,
            retryStrategy: $this->retryStrategy,
            scheduledFor: $this->scheduledFor,
            expiresAt: $this->expiresAt,
            idempotencyKey: $this->idempotencyKey,
            externalId: $this->externalId,
        );
    }

    /**
     * Add a subject.
     */
    public function withSubject(string $subject): self
    {
        return new self(
            source: $this->source,
            code: $this->code,
            targetUrl: $this->targetUrl,
            payload: $this->payload,
            dispatchPoolId: $this->dispatchPoolId,
            partitionId: $this->partitionId,
            subject: $subject,
            correlationId: $this->correlationId,
            eventId: $this->eventId,
            metadata: $this->metadata,
            headers: $this->headers,
            payloadContentType: $this->payloadContentType,
            dataOnly: $this->dataOnly,
            messageGroup: $this->messageGroup,
            sequence: $this->sequence,
            timeoutSeconds: $this->timeoutSeconds,
            maxRetries: $this->maxRetries,
            retryStrategy: $this->retryStrategy,
            scheduledFor: $this->scheduledFor,
            expiresAt: $this->expiresAt,
            idempotencyKey: $this->idempotencyKey,
            externalId: $this->externalId,
        );
    }

    /**
     * Add HTTP headers for the webhook.
     */
    public function withHeaders(array $headers): self
    {
        return new self(
            source: $this->source,
            code: $this->code,
            targetUrl: $this->targetUrl,
            payload: $this->payload,
            dispatchPoolId: $this->dispatchPoolId,
            partitionId: $this->partitionId,
            subject: $this->subject,
            correlationId: $this->correlationId,
            eventId: $this->eventId,
            metadata: $this->metadata,
            headers: array_merge($this->headers, $headers),
            payloadContentType: $this->payloadContentType,
            dataOnly: $this->dataOnly,
            messageGroup: $this->messageGroup,
            sequence: $this->sequence,
            timeoutSeconds: $this->timeoutSeconds,
            maxRetries: $this->maxRetries,
            retryStrategy: $this->retryStrategy,
            scheduledFor: $this->scheduledFor,
            expiresAt: $this->expiresAt,
            idempotencyKey: $this->idempotencyKey,
            externalId: $this->externalId,
        );
    }

    /**
     * Add metadata.
     */
    public function withMetadata(array $metadata): self
    {
        return new self(
            source: $this->source,
            code: $this->code,
            targetUrl: $this->targetUrl,
            payload: $this->payload,
            dispatchPoolId: $this->dispatchPoolId,
            partitionId: $this->partitionId,
            subject: $this->subject,
            correlationId: $this->correlationId,
            eventId: $this->eventId,
            metadata: array_merge($this->metadata, $metadata),
            headers: $this->headers,
            payloadContentType: $this->payloadContentType,
            dataOnly: $this->dataOnly,
            messageGroup: $this->messageGroup,
            sequence: $this->sequence,
            timeoutSeconds: $this->timeoutSeconds,
            maxRetries: $this->maxRetries,
            retryStrategy: $this->retryStrategy,
            scheduledFor: $this->scheduledFor,
            expiresAt: $this->expiresAt,
            idempotencyKey: $this->idempotencyKey,
            externalId: $this->externalId,
        );
    }

    /**
     * Schedule the job for later execution.
     */
    public function scheduledFor(\DateTimeInterface $scheduledFor): self
    {
        return new self(
            source: $this->source,
            code: $this->code,
            targetUrl: $this->targetUrl,
            payload: $this->payload,
            dispatchPoolId: $this->dispatchPoolId,
            partitionId: $this->partitionId,
            subject: $this->subject,
            correlationId: $this->correlationId,
            eventId: $this->eventId,
            metadata: $this->metadata,
            headers: $this->headers,
            payloadContentType: $this->payloadContentType,
            dataOnly: $this->dataOnly,
            messageGroup: $this->messageGroup,
            sequence: $this->sequence,
            timeoutSeconds: $this->timeoutSeconds,
            maxRetries: $this->maxRetries,
            retryStrategy: $this->retryStrategy,
            scheduledFor: $scheduledFor,
            expiresAt: $this->expiresAt,
            idempotencyKey: $this->idempotencyKey,
            externalId: $this->externalId,
        );
    }

    /**
     * Set an expiration time.
     */
    public function expiresAt(\DateTimeInterface $expiresAt): self
    {
        return new self(
            source: $this->source,
            code: $this->code,
            targetUrl: $this->targetUrl,
            payload: $this->payload,
            dispatchPoolId: $this->dispatchPoolId,
            partitionId: $this->partitionId,
            subject: $this->subject,
            correlationId: $this->correlationId,
            eventId: $this->eventId,
            metadata: $this->metadata,
            headers: $this->headers,
            payloadContentType: $this->payloadContentType,
            dataOnly: $this->dataOnly,
            messageGroup: $this->messageGroup,
            sequence: $this->sequence,
            timeoutSeconds: $this->timeoutSeconds,
            maxRetries: $this->maxRetries,
            retryStrategy: $this->retryStrategy,
            scheduledFor: $this->scheduledFor,
            expiresAt: $expiresAt,
            idempotencyKey: $this->idempotencyKey,
            externalId: $this->externalId,
        );
    }

    /**
     * Set an idempotency key.
     */
    public function withIdempotencyKey(string $idempotencyKey): self
    {
        return new self(
            source: $this->source,
            code: $this->code,
            targetUrl: $this->targetUrl,
            payload: $this->payload,
            dispatchPoolId: $this->dispatchPoolId,
            partitionId: $this->partitionId,
            subject: $this->subject,
            correlationId: $this->correlationId,
            eventId: $this->eventId,
            metadata: $this->metadata,
            headers: $this->headers,
            payloadContentType: $this->payloadContentType,
            dataOnly: $this->dataOnly,
            messageGroup: $this->messageGroup,
            sequence: $this->sequence,
            timeoutSeconds: $this->timeoutSeconds,
            maxRetries: $this->maxRetries,
            retryStrategy: $this->retryStrategy,
            scheduledFor: $this->scheduledFor,
            expiresAt: $this->expiresAt,
            idempotencyKey: $idempotencyKey,
            externalId: $this->externalId,
        );
    }

    /**
     * Build the dispatch job payload for the outbox.
     */
    public function toPayload(): array
    {
        return array_filter([
            'source' => $this->source,
            'code' => $this->code,
            'targetUrl' => $this->targetUrl,
            'payload' => $this->payload,
            'payloadContentType' => $this->payloadContentType,
            'dispatchPoolId' => $this->dispatchPoolId,
            'subject' => $this->subject,
            'correlationId' => $this->correlationId,
            'eventId' => $this->eventId,
            'metadata' => !empty($this->metadata) ? $this->metadata : null,
            'headers' => !empty($this->headers) ? $this->headers : null,
            'dataOnly' => $this->dataOnly,
            'messageGroup' => $this->messageGroup,
            'sequence' => $this->sequence,
            'timeoutSeconds' => $this->timeoutSeconds,
            'maxRetries' => $this->maxRetries,
            'retryStrategy' => $this->retryStrategy,
            'scheduledFor' => $this->scheduledFor?->format('c'),
            'expiresAt' => $this->expiresAt?->format('c'),
            'idempotencyKey' => $this->idempotencyKey,
            'externalId' => $this->externalId,
        ], fn($v) => $v !== null);
    }
}
