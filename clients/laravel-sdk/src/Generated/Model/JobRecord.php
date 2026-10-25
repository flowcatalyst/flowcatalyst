<?php

namespace FlowCatalyst\Generated\Model;

class JobRecord
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
     * At most 100 characters
     *
     * @var string|null
     */
    protected $correlationId;
    /**
     * The record's idempotency key within the ingestion, at most 100 characters (the runtime sends the sha256 hex of the record's source identity, 64): sending it again answers the job it made first, and of two records in one request with the same key the first makes the job
     *
     * @var string|null
     */
    protected $externalId;
    /**
     * Orders the jobs of a group within the target's mode; at most 200 characters
     *
     * @var string|null
     */
    protected $messageGroup;
    /**
     * The record, as text
     *
     * @var string|null
     */
    protected $payload;
    /**
     * Defaults to application/json; at most 100 characters
     *
     * @var string|null
     */
    protected $payloadContentType;
    /**
     * At most 500 characters
     *
     * @var string|null
     */
    protected $subject;
    /**
     * At most 100 characters
     *
     * @return string|null
     */
    public function getCorrelationId(): ?string
    {
        return $this->correlationId;
    }
    /**
     * At most 100 characters
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
     * The record's idempotency key within the ingestion, at most 100 characters (the runtime sends the sha256 hex of the record's source identity, 64): sending it again answers the job it made first, and of two records in one request with the same key the first makes the job
     *
     * @return string|null
     */
    public function getExternalId(): ?string
    {
        return $this->externalId;
    }
    /**
     * The record's idempotency key within the ingestion, at most 100 characters (the runtime sends the sha256 hex of the record's source identity, 64): sending it again answers the job it made first, and of two records in one request with the same key the first makes the job
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
     * Orders the jobs of a group within the target's mode; at most 200 characters
     *
     * @return string|null
     */
    public function getMessageGroup(): ?string
    {
        return $this->messageGroup;
    }
    /**
     * Orders the jobs of a group within the target's mode; at most 200 characters
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
     * The record, as text
     *
     * @return string|null
     */
    public function getPayload(): ?string
    {
        return $this->payload;
    }
    /**
     * The record, as text
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
     * Defaults to application/json; at most 100 characters
     *
     * @return string|null
     */
    public function getPayloadContentType(): ?string
    {
        return $this->payloadContentType;
    }
    /**
     * Defaults to application/json; at most 100 characters
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
     * At most 500 characters
     *
     * @return string|null
     */
    public function getSubject(): ?string
    {
        return $this->subject;
    }
    /**
     * At most 500 characters
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
}