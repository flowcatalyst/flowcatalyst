<?php

namespace FlowCatalyst\Generated\Model;

class CreateDispatchJobRequest extends \ArrayObject
{
    /**
     * @var array
     */
    protected $initialized = [];
    public function isInitialized($property): bool
    {
        return array_key_exists($property, $this->initialized);
    }
    /**
     * Client ID
     *
     * @var string|null
     */
    protected $clientId;
    /**
     * The event type or task code
     *
     * @var string|null
     */
    protected $code;
    /**
     * Correlation ID for distributed tracing
     *
     * @var string|null
     */
    protected $correlationId;
    /**
     * If true, send raw payload only
     *
     * @var bool|null
     */
    protected $dataOnly;
    /**
     * Rate limiting pool ID
     *
     * @var string|null
     */
    protected $dispatchPoolId;
    /**
     * Source event ID (required for EVENT kind)
     *
     * @var string|null
     */
    protected $eventId;
    /**
     * External reference ID
     *
     * @var string|null
     */
    protected $externalId;
    /**
     * Idempotency key for deduplication
     *
     * @var string|null
     */
    protected $idempotencyKey;
    /**
     * The kind of dispatch job (EVENT or TASK)
     *
     * @var string|null
     */
    protected $kind;
    /**
     * Maximum retry attempts
     *
     * @var int|null
     */
    protected $maxRetries;
    /**
     * Message group for FIFO ordering
     *
     * @var string|null
     */
    protected $messageGroup;
    /**
     * Custom metadata
     *
     * @var array<string, string>|null
     */
    protected $metadata;
    /**
     * Dispatch mode for ordering
     *
     * @var string|null
     */
    protected $mode;
    /**
     * Payload to deliver (JSON string)
     *
     * @var string|null
     */
    protected $payload;
    /**
     * Content type of payload
     *
     * @var string|null
     */
    protected $payloadContentType;
    /**
     * Retry strategy
     *
     * @var string|null
     */
    protected $retryStrategy;
    /**
     * Sequence number within message group
     *
     * @var int|null
     */
    protected $sequence;
    /**
     * Service account for authentication
     *
     * @var string|null
     */
    protected $serviceAccountId;
    /**
     * Source system/application
     *
     * @var string|null
     */
    protected $source;
    /**
     * CloudEvents-style subject/aggregate reference
     *
     * @var string|null
     */
    protected $subject;
    /**
     * Subscription ID that created this job
     *
     * @var string|null
     */
    protected $subscriptionId;
    /**
     * Target URL for webhook delivery
     *
     * @var string|null
     */
    protected $targetUrl;
    /**
     * Timeout in seconds for HTTP call
     *
     * @var int|null
     */
    protected $timeoutSeconds;
    /**
     * Client ID
     *
     * @return string|null
     */
    public function getClientId(): ?string
    {
        return $this->clientId;
    }
    /**
     * Client ID
     *
     * @param string|null $clientId
     *
     * @return self
     */
    public function setClientId(?string $clientId): self
    {
        $this->initialized['clientId'] = true;
        $this->clientId = $clientId;
        return $this;
    }
    /**
     * The event type or task code
     *
     * @return string|null
     */
    public function getCode(): ?string
    {
        return $this->code;
    }
    /**
     * The event type or task code
     *
     * @param string|null $code
     *
     * @return self
     */
    public function setCode(?string $code): self
    {
        $this->initialized['code'] = true;
        $this->code = $code;
        return $this;
    }
    /**
     * Correlation ID for distributed tracing
     *
     * @return string|null
     */
    public function getCorrelationId(): ?string
    {
        return $this->correlationId;
    }
    /**
     * Correlation ID for distributed tracing
     *
     * @param string|null $correlationId
     *
     * @return self
     */
    public function setCorrelationId(?string $correlationId): self
    {
        $this->initialized['correlationId'] = true;
        $this->correlationId = $correlationId;
        return $this;
    }
    /**
     * If true, send raw payload only
     *
     * @return bool|null
     */
    public function getDataOnly(): ?bool
    {
        return $this->dataOnly;
    }
    /**
     * If true, send raw payload only
     *
     * @param bool|null $dataOnly
     *
     * @return self
     */
    public function setDataOnly(?bool $dataOnly): self
    {
        $this->initialized['dataOnly'] = true;
        $this->dataOnly = $dataOnly;
        return $this;
    }
    /**
     * Rate limiting pool ID
     *
     * @return string|null
     */
    public function getDispatchPoolId(): ?string
    {
        return $this->dispatchPoolId;
    }
    /**
     * Rate limiting pool ID
     *
     * @param string|null $dispatchPoolId
     *
     * @return self
     */
    public function setDispatchPoolId(?string $dispatchPoolId): self
    {
        $this->initialized['dispatchPoolId'] = true;
        $this->dispatchPoolId = $dispatchPoolId;
        return $this;
    }
    /**
     * Source event ID (required for EVENT kind)
     *
     * @return string|null
     */
    public function getEventId(): ?string
    {
        return $this->eventId;
    }
    /**
     * Source event ID (required for EVENT kind)
     *
     * @param string|null $eventId
     *
     * @return self
     */
    public function setEventId(?string $eventId): self
    {
        $this->initialized['eventId'] = true;
        $this->eventId = $eventId;
        return $this;
    }
    /**
     * External reference ID
     *
     * @return string|null
     */
    public function getExternalId(): ?string
    {
        return $this->externalId;
    }
    /**
     * External reference ID
     *
     * @param string|null $externalId
     *
     * @return self
     */
    public function setExternalId(?string $externalId): self
    {
        $this->initialized['externalId'] = true;
        $this->externalId = $externalId;
        return $this;
    }
    /**
     * Idempotency key for deduplication
     *
     * @return string|null
     */
    public function getIdempotencyKey(): ?string
    {
        return $this->idempotencyKey;
    }
    /**
     * Idempotency key for deduplication
     *
     * @param string|null $idempotencyKey
     *
     * @return self
     */
    public function setIdempotencyKey(?string $idempotencyKey): self
    {
        $this->initialized['idempotencyKey'] = true;
        $this->idempotencyKey = $idempotencyKey;
        return $this;
    }
    /**
     * The kind of dispatch job (EVENT or TASK)
     *
     * @return string|null
     */
    public function getKind(): ?string
    {
        return $this->kind;
    }
    /**
     * The kind of dispatch job (EVENT or TASK)
     *
     * @param string|null $kind
     *
     * @return self
     */
    public function setKind(?string $kind): self
    {
        $this->initialized['kind'] = true;
        $this->kind = $kind;
        return $this;
    }
    /**
     * Maximum retry attempts
     *
     * @return int|null
     */
    public function getMaxRetries(): ?int
    {
        return $this->maxRetries;
    }
    /**
     * Maximum retry attempts
     *
     * @param int|null $maxRetries
     *
     * @return self
     */
    public function setMaxRetries(?int $maxRetries): self
    {
        $this->initialized['maxRetries'] = true;
        $this->maxRetries = $maxRetries;
        return $this;
    }
    /**
     * Message group for FIFO ordering
     *
     * @return string|null
     */
    public function getMessageGroup(): ?string
    {
        return $this->messageGroup;
    }
    /**
     * Message group for FIFO ordering
     *
     * @param string|null $messageGroup
     *
     * @return self
     */
    public function setMessageGroup(?string $messageGroup): self
    {
        $this->initialized['messageGroup'] = true;
        $this->messageGroup = $messageGroup;
        return $this;
    }
    /**
     * Custom metadata
     *
     * @return array<string, string>|null
     */
    public function getMetadata(): ?iterable
    {
        return $this->metadata;
    }
    /**
     * Custom metadata
     *
     * @param array<string, string>|null $metadata
     *
     * @return self
     */
    public function setMetadata(?iterable $metadata): self
    {
        $this->initialized['metadata'] = true;
        $this->metadata = $metadata;
        return $this;
    }
    /**
     * Dispatch mode for ordering
     *
     * @return string|null
     */
    public function getMode(): ?string
    {
        return $this->mode;
    }
    /**
     * Dispatch mode for ordering
     *
     * @param string|null $mode
     *
     * @return self
     */
    public function setMode(?string $mode): self
    {
        $this->initialized['mode'] = true;
        $this->mode = $mode;
        return $this;
    }
    /**
     * Payload to deliver (JSON string)
     *
     * @return string|null
     */
    public function getPayload(): ?string
    {
        return $this->payload;
    }
    /**
     * Payload to deliver (JSON string)
     *
     * @param string|null $payload
     *
     * @return self
     */
    public function setPayload(?string $payload): self
    {
        $this->initialized['payload'] = true;
        $this->payload = $payload;
        return $this;
    }
    /**
     * Content type of payload
     *
     * @return string|null
     */
    public function getPayloadContentType(): ?string
    {
        return $this->payloadContentType;
    }
    /**
     * Content type of payload
     *
     * @param string|null $payloadContentType
     *
     * @return self
     */
    public function setPayloadContentType(?string $payloadContentType): self
    {
        $this->initialized['payloadContentType'] = true;
        $this->payloadContentType = $payloadContentType;
        return $this;
    }
    /**
     * Retry strategy
     *
     * @return string|null
     */
    public function getRetryStrategy(): ?string
    {
        return $this->retryStrategy;
    }
    /**
     * Retry strategy
     *
     * @param string|null $retryStrategy
     *
     * @return self
     */
    public function setRetryStrategy(?string $retryStrategy): self
    {
        $this->initialized['retryStrategy'] = true;
        $this->retryStrategy = $retryStrategy;
        return $this;
    }
    /**
     * Sequence number within message group
     *
     * @return int|null
     */
    public function getSequence(): ?int
    {
        return $this->sequence;
    }
    /**
     * Sequence number within message group
     *
     * @param int|null $sequence
     *
     * @return self
     */
    public function setSequence(?int $sequence): self
    {
        $this->initialized['sequence'] = true;
        $this->sequence = $sequence;
        return $this;
    }
    /**
     * Service account for authentication
     *
     * @return string|null
     */
    public function getServiceAccountId(): ?string
    {
        return $this->serviceAccountId;
    }
    /**
     * Service account for authentication
     *
     * @param string|null $serviceAccountId
     *
     * @return self
     */
    public function setServiceAccountId(?string $serviceAccountId): self
    {
        $this->initialized['serviceAccountId'] = true;
        $this->serviceAccountId = $serviceAccountId;
        return $this;
    }
    /**
     * Source system/application
     *
     * @return string|null
     */
    public function getSource(): ?string
    {
        return $this->source;
    }
    /**
     * Source system/application
     *
     * @param string|null $source
     *
     * @return self
     */
    public function setSource(?string $source): self
    {
        $this->initialized['source'] = true;
        $this->source = $source;
        return $this;
    }
    /**
     * CloudEvents-style subject/aggregate reference
     *
     * @return string|null
     */
    public function getSubject(): ?string
    {
        return $this->subject;
    }
    /**
     * CloudEvents-style subject/aggregate reference
     *
     * @param string|null $subject
     *
     * @return self
     */
    public function setSubject(?string $subject): self
    {
        $this->initialized['subject'] = true;
        $this->subject = $subject;
        return $this;
    }
    /**
     * Subscription ID that created this job
     *
     * @return string|null
     */
    public function getSubscriptionId(): ?string
    {
        return $this->subscriptionId;
    }
    /**
     * Subscription ID that created this job
     *
     * @param string|null $subscriptionId
     *
     * @return self
     */
    public function setSubscriptionId(?string $subscriptionId): self
    {
        $this->initialized['subscriptionId'] = true;
        $this->subscriptionId = $subscriptionId;
        return $this;
    }
    /**
     * Target URL for webhook delivery
     *
     * @return string|null
     */
    public function getTargetUrl(): ?string
    {
        return $this->targetUrl;
    }
    /**
     * Target URL for webhook delivery
     *
     * @param string|null $targetUrl
     *
     * @return self
     */
    public function setTargetUrl(?string $targetUrl): self
    {
        $this->initialized['targetUrl'] = true;
        $this->targetUrl = $targetUrl;
        return $this;
    }
    /**
     * Timeout in seconds for HTTP call
     *
     * @return int|null
     */
    public function getTimeoutSeconds(): ?int
    {
        return $this->timeoutSeconds;
    }
    /**
     * Timeout in seconds for HTTP call
     *
     * @param int|null $timeoutSeconds
     *
     * @return self
     */
    public function setTimeoutSeconds(?int $timeoutSeconds): self
    {
        $this->initialized['timeoutSeconds'] = true;
        $this->timeoutSeconds = $timeoutSeconds;
        return $this;
    }
}