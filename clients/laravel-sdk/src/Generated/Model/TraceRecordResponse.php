<?php

namespace FlowCatalyst\Generated\Model;

class TraceRecordResponse
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
     * @var \DateTime|null
     */
    protected $at;
    /**
     * @var bool|null
     */
    protected $bodiesRedacted;
    /**
     * @var string|null
     */
    protected $context;
    /**
     * @var string|null
     */
    protected $correlationId;
    /**
     * The record's data. For kind http: request, response, durationMs, outcome.
     *
     * @var mixed|null
     */
    protected $data;
    /**
     * @var string|null
     */
    protected $id;
    /**
     * @var string|null
     */
    protected $invocationId;
    /**
     * @var string|null
     */
    protected $kind;
    /**
     * @var string|null
     */
    protected $level;
    /**
     * @var string|null
     */
    protected $message;
    /**
     * @var string|null
     */
    protected $reference;
    /**
     * @var string|null
     */
    protected $runnerId;
    /**
     * @var int|null
     */
    protected $version;
    /**
     * @return \DateTime|null
     */
    public function getAt(): ?\DateTime
    {
        return $this->at;
    }
    /**
     * @param \DateTime|null $at
     *
     * @return self
     */
    public function setAt(?\DateTime $at): self
    {
        $this->initialized['at'] = true;
        $this->at = $at;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getBodiesRedacted(): ?bool
    {
        return $this->bodiesRedacted;
    }
    /**
     * @param bool|null $bodiesRedacted
     *
     * @return self
     */
    public function setBodiesRedacted(?bool $bodiesRedacted): self
    {
        $this->initialized['bodiesRedacted'] = true;
        $this->bodiesRedacted = $bodiesRedacted;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getContext(): ?string
    {
        return $this->context;
    }
    /**
     * @param string|null $context
     *
     * @return self
     */
    public function setContext(?string $context): self
    {
        $this->initialized['context'] = true;
        $this->context = $context;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getCorrelationId(): ?string
    {
        return $this->correlationId;
    }
    /**
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
     * The record's data. For kind http: request, response, durationMs, outcome.
     *
     * @return mixed
     */
    public function getData()
    {
        return $this->data;
    }
    /**
     * The record's data. For kind http: request, response, durationMs, outcome.
     *
     * @param mixed $data
     *
     * @return self
     */
    public function setData($data): self
    {
        $this->initialized['data'] = true;
        $this->data = $data;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getId(): ?string
    {
        return $this->id;
    }
    /**
     * @param string|null $id
     *
     * @return self
     */
    public function setId(?string $id): self
    {
        $this->initialized['id'] = true;
        $this->id = $id;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getInvocationId(): ?string
    {
        return $this->invocationId;
    }
    /**
     * @param string|null $invocationId
     *
     * @return self
     */
    public function setInvocationId(?string $invocationId): self
    {
        $this->initialized['invocationId'] = true;
        $this->invocationId = $invocationId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getKind(): ?string
    {
        return $this->kind;
    }
    /**
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
     * @return string|null
     */
    public function getLevel(): ?string
    {
        return $this->level;
    }
    /**
     * @param string|null $level
     *
     * @return self
     */
    public function setLevel(?string $level): self
    {
        $this->initialized['level'] = true;
        $this->level = $level;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getMessage(): ?string
    {
        return $this->message;
    }
    /**
     * @param string|null $message
     *
     * @return self
     */
    public function setMessage(?string $message): self
    {
        $this->initialized['message'] = true;
        $this->message = $message;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getReference(): ?string
    {
        return $this->reference;
    }
    /**
     * @param string|null $reference
     *
     * @return self
     */
    public function setReference(?string $reference): self
    {
        $this->initialized['reference'] = true;
        $this->reference = $reference;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getRunnerId(): ?string
    {
        return $this->runnerId;
    }
    /**
     * @param string|null $runnerId
     *
     * @return self
     */
    public function setRunnerId(?string $runnerId): self
    {
        $this->initialized['runnerId'] = true;
        $this->runnerId = $runnerId;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getVersion(): ?int
    {
        return $this->version;
    }
    /**
     * @param int|null $version
     *
     * @return self
     */
    public function setVersion(?int $version): self
    {
        $this->initialized['version'] = true;
        $this->version = $version;
        return $this;
    }
}