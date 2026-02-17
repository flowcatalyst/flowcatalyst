<?php

namespace FlowCatalyst\Generated\Model;

class ApiBffDebugEventsIdGetResponse200 extends \ArrayObject
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
     * @var mixed|null
     */
    protected $subject;
    /**
     * @var \DateTime|null
     */
    protected $time;
    /**
     * @var mixed|null
     */
    protected $data;
    /**
     * @var mixed|null
     */
    protected $messageGroup;
    /**
     * @var mixed|null
     */
    protected $correlationId;
    /**
     * @var mixed|null
     */
    protected $causationId;
    /**
     * @var mixed|null
     */
    protected $deduplicationId;
    /**
     * @var mixed|null
     */
    protected $contextData;
    /**
     * @var mixed|null
     */
    protected $clientId;
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
     * @return mixed
     */
    public function getSubject()
    {
        return $this->subject;
    }
    /**
     * @param mixed $subject
     *
     * @return self
     */
    public function setSubject($subject): self
    {
        $this->initialized['subject'] = true;
        $this->subject = $subject;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getTime(): ?\DateTime
    {
        return $this->time;
    }
    /**
     * @param \DateTime|null $time
     *
     * @return self
     */
    public function setTime(?\DateTime $time): self
    {
        $this->initialized['time'] = true;
        $this->time = $time;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getData()
    {
        return $this->data;
    }
    /**
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
     * @return mixed
     */
    public function getMessageGroup()
    {
        return $this->messageGroup;
    }
    /**
     * @param mixed $messageGroup
     *
     * @return self
     */
    public function setMessageGroup($messageGroup): self
    {
        $this->initialized['messageGroup'] = true;
        $this->messageGroup = $messageGroup;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getCorrelationId()
    {
        return $this->correlationId;
    }
    /**
     * @param mixed $correlationId
     *
     * @return self
     */
    public function setCorrelationId($correlationId): self
    {
        $this->initialized['correlationId'] = true;
        $this->correlationId = $correlationId;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getCausationId()
    {
        return $this->causationId;
    }
    /**
     * @param mixed $causationId
     *
     * @return self
     */
    public function setCausationId($causationId): self
    {
        $this->initialized['causationId'] = true;
        $this->causationId = $causationId;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getDeduplicationId()
    {
        return $this->deduplicationId;
    }
    /**
     * @param mixed $deduplicationId
     *
     * @return self
     */
    public function setDeduplicationId($deduplicationId): self
    {
        $this->initialized['deduplicationId'] = true;
        $this->deduplicationId = $deduplicationId;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getContextData()
    {
        return $this->contextData;
    }
    /**
     * @param mixed $contextData
     *
     * @return self
     */
    public function setContextData($contextData): self
    {
        $this->initialized['contextData'] = true;
        $this->contextData = $contextData;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getClientId()
    {
        return $this->clientId;
    }
    /**
     * @param mixed $clientId
     *
     * @return self
     */
    public function setClientId($clientId): self
    {
        $this->initialized['clientId'] = true;
        $this->clientId = $clientId;
        return $this;
    }
}