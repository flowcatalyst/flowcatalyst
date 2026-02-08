<?php

namespace FlowCatalyst\Generated\Model;

class EventResponse extends \ArrayObject
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
     * @var string|null
     */
    protected $id;
    /**
     * @var string|null
     */
    protected $specVersion;
    /**
     * @var string|null
     */
    protected $type;
    /**
     * @var string|null
     */
    protected $source;
    /**
     * @var string|null
     */
    protected $subject;
    /**
     * @var string|null
     */
    protected $time;
    /**
     * @var string|null
     */
    protected $data;
    /**
     * @var string|null
     */
    protected $correlationId;
    /**
     * @var string|null
     */
    protected $causationId;
    /**
     * @var string|null
     */
    protected $deduplicationId;
    /**
     * @var string|null
     */
    protected $messageGroup;
    /**
     * @var list<ContextDataResponse>|null
     */
    protected $contextData;
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
    public function getSpecVersion(): ?string
    {
        return $this->specVersion;
    }
    /**
     * @param string|null $specVersion
     *
     * @return self
     */
    public function setSpecVersion(?string $specVersion): self
    {
        $this->initialized['specVersion'] = true;
        $this->specVersion = $specVersion;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getType(): ?string
    {
        return $this->type;
    }
    /**
     * @param string|null $type
     *
     * @return self
     */
    public function setType(?string $type): self
    {
        $this->initialized['type'] = true;
        $this->type = $type;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getSource(): ?string
    {
        return $this->source;
    }
    /**
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
     * @return string|null
     */
    public function getSubject(): ?string
    {
        return $this->subject;
    }
    /**
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
     * @return string|null
     */
    public function getTime(): ?string
    {
        return $this->time;
    }
    /**
     * @param string|null $time
     *
     * @return self
     */
    public function setTime(?string $time): self
    {
        $this->initialized['time'] = true;
        $this->time = $time;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getData(): ?string
    {
        return $this->data;
    }
    /**
     * @param string|null $data
     *
     * @return self
     */
    public function setData(?string $data): self
    {
        $this->initialized['data'] = true;
        $this->data = $data;
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
     * @return string|null
     */
    public function getCausationId(): ?string
    {
        return $this->causationId;
    }
    /**
     * @param string|null $causationId
     *
     * @return self
     */
    public function setCausationId(?string $causationId): self
    {
        $this->initialized['causationId'] = true;
        $this->causationId = $causationId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getDeduplicationId(): ?string
    {
        return $this->deduplicationId;
    }
    /**
     * @param string|null $deduplicationId
     *
     * @return self
     */
    public function setDeduplicationId(?string $deduplicationId): self
    {
        $this->initialized['deduplicationId'] = true;
        $this->deduplicationId = $deduplicationId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getMessageGroup(): ?string
    {
        return $this->messageGroup;
    }
    /**
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
     * @return list<ContextDataResponse>|null
     */
    public function getContextData(): ?array
    {
        return $this->contextData;
    }
    /**
     * @param list<ContextDataResponse>|null $contextData
     *
     * @return self
     */
    public function setContextData(?array $contextData): self
    {
        $this->initialized['contextData'] = true;
        $this->contextData = $contextData;
        return $this;
    }
}