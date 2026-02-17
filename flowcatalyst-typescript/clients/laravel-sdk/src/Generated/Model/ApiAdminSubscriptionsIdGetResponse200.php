<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminSubscriptionsIdGetResponse200 extends \ArrayObject
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
    protected $code;
    /**
     * @var mixed|null
     */
    protected $applicationCode;
    /**
     * @var string|null
     */
    protected $name;
    /**
     * @var mixed|null
     */
    protected $description;
    /**
     * @var mixed|null
     */
    protected $clientId;
    /**
     * @var mixed|null
     */
    protected $clientIdentifier;
    /**
     * @var bool|null
     */
    protected $clientScoped;
    /**
     * @var list<ApiAdminSubscriptionsIdGetResponse200EventTypesItem>|null
     */
    protected $eventTypes;
    /**
     * @var string|null
     */
    protected $target;
    /**
     * @var mixed|null
     */
    protected $queue;
    /**
     * @var list<ApiAdminSubscriptionsIdGetResponse200CustomConfigItem>|null
     */
    protected $customConfig;
    /**
     * @var string|null
     */
    protected $source;
    /**
     * @var string|null
     */
    protected $status;
    /**
     * @var int|null
     */
    protected $maxAgeSeconds;
    /**
     * @var mixed|null
     */
    protected $dispatchPoolId;
    /**
     * @var mixed|null
     */
    protected $dispatchPoolCode;
    /**
     * @var int|null
     */
    protected $delaySeconds;
    /**
     * @var int|null
     */
    protected $sequence;
    /**
     * @var string|null
     */
    protected $mode;
    /**
     * @var int|null
     */
    protected $timeoutSeconds;
    /**
     * @var int|null
     */
    protected $maxRetries;
    /**
     * @var mixed|null
     */
    protected $serviceAccountId;
    /**
     * @var bool|null
     */
    protected $dataOnly;
    /**
     * @var \DateTime|null
     */
    protected $createdAt;
    /**
     * @var \DateTime|null
     */
    protected $updatedAt;
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
    public function getCode(): ?string
    {
        return $this->code;
    }
    /**
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
     * @return mixed
     */
    public function getApplicationCode()
    {
        return $this->applicationCode;
    }
    /**
     * @param mixed $applicationCode
     *
     * @return self
     */
    public function setApplicationCode($applicationCode): self
    {
        $this->initialized['applicationCode'] = true;
        $this->applicationCode = $applicationCode;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getName(): ?string
    {
        return $this->name;
    }
    /**
     * @param string|null $name
     *
     * @return self
     */
    public function setName(?string $name): self
    {
        $this->initialized['name'] = true;
        $this->name = $name;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getDescription()
    {
        return $this->description;
    }
    /**
     * @param mixed $description
     *
     * @return self
     */
    public function setDescription($description): self
    {
        $this->initialized['description'] = true;
        $this->description = $description;
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
    /**
     * @return mixed
     */
    public function getClientIdentifier()
    {
        return $this->clientIdentifier;
    }
    /**
     * @param mixed $clientIdentifier
     *
     * @return self
     */
    public function setClientIdentifier($clientIdentifier): self
    {
        $this->initialized['clientIdentifier'] = true;
        $this->clientIdentifier = $clientIdentifier;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getClientScoped(): ?bool
    {
        return $this->clientScoped;
    }
    /**
     * @param bool|null $clientScoped
     *
     * @return self
     */
    public function setClientScoped(?bool $clientScoped): self
    {
        $this->initialized['clientScoped'] = true;
        $this->clientScoped = $clientScoped;
        return $this;
    }
    /**
     * @return list<ApiAdminSubscriptionsIdGetResponse200EventTypesItem>|null
     */
    public function getEventTypes(): ?array
    {
        return $this->eventTypes;
    }
    /**
     * @param list<ApiAdminSubscriptionsIdGetResponse200EventTypesItem>|null $eventTypes
     *
     * @return self
     */
    public function setEventTypes(?array $eventTypes): self
    {
        $this->initialized['eventTypes'] = true;
        $this->eventTypes = $eventTypes;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getTarget(): ?string
    {
        return $this->target;
    }
    /**
     * @param string|null $target
     *
     * @return self
     */
    public function setTarget(?string $target): self
    {
        $this->initialized['target'] = true;
        $this->target = $target;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getQueue()
    {
        return $this->queue;
    }
    /**
     * @param mixed $queue
     *
     * @return self
     */
    public function setQueue($queue): self
    {
        $this->initialized['queue'] = true;
        $this->queue = $queue;
        return $this;
    }
    /**
     * @return list<ApiAdminSubscriptionsIdGetResponse200CustomConfigItem>|null
     */
    public function getCustomConfig(): ?array
    {
        return $this->customConfig;
    }
    /**
     * @param list<ApiAdminSubscriptionsIdGetResponse200CustomConfigItem>|null $customConfig
     *
     * @return self
     */
    public function setCustomConfig(?array $customConfig): self
    {
        $this->initialized['customConfig'] = true;
        $this->customConfig = $customConfig;
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
    public function getStatus(): ?string
    {
        return $this->status;
    }
    /**
     * @param string|null $status
     *
     * @return self
     */
    public function setStatus(?string $status): self
    {
        $this->initialized['status'] = true;
        $this->status = $status;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getMaxAgeSeconds(): ?int
    {
        return $this->maxAgeSeconds;
    }
    /**
     * @param int|null $maxAgeSeconds
     *
     * @return self
     */
    public function setMaxAgeSeconds(?int $maxAgeSeconds): self
    {
        $this->initialized['maxAgeSeconds'] = true;
        $this->maxAgeSeconds = $maxAgeSeconds;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getDispatchPoolId()
    {
        return $this->dispatchPoolId;
    }
    /**
     * @param mixed $dispatchPoolId
     *
     * @return self
     */
    public function setDispatchPoolId($dispatchPoolId): self
    {
        $this->initialized['dispatchPoolId'] = true;
        $this->dispatchPoolId = $dispatchPoolId;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getDispatchPoolCode()
    {
        return $this->dispatchPoolCode;
    }
    /**
     * @param mixed $dispatchPoolCode
     *
     * @return self
     */
    public function setDispatchPoolCode($dispatchPoolCode): self
    {
        $this->initialized['dispatchPoolCode'] = true;
        $this->dispatchPoolCode = $dispatchPoolCode;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getDelaySeconds(): ?int
    {
        return $this->delaySeconds;
    }
    /**
     * @param int|null $delaySeconds
     *
     * @return self
     */
    public function setDelaySeconds(?int $delaySeconds): self
    {
        $this->initialized['delaySeconds'] = true;
        $this->delaySeconds = $delaySeconds;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getSequence(): ?int
    {
        return $this->sequence;
    }
    /**
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
     * @return string|null
     */
    public function getMode(): ?string
    {
        return $this->mode;
    }
    /**
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
     * @return int|null
     */
    public function getTimeoutSeconds(): ?int
    {
        return $this->timeoutSeconds;
    }
    /**
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
    /**
     * @return int|null
     */
    public function getMaxRetries(): ?int
    {
        return $this->maxRetries;
    }
    /**
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
     * @return mixed
     */
    public function getServiceAccountId()
    {
        return $this->serviceAccountId;
    }
    /**
     * @param mixed $serviceAccountId
     *
     * @return self
     */
    public function setServiceAccountId($serviceAccountId): self
    {
        $this->initialized['serviceAccountId'] = true;
        $this->serviceAccountId = $serviceAccountId;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getDataOnly(): ?bool
    {
        return $this->dataOnly;
    }
    /**
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
     * @return \DateTime|null
     */
    public function getCreatedAt(): ?\DateTime
    {
        return $this->createdAt;
    }
    /**
     * @param \DateTime|null $createdAt
     *
     * @return self
     */
    public function setCreatedAt(?\DateTime $createdAt): self
    {
        $this->initialized['createdAt'] = true;
        $this->createdAt = $createdAt;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getUpdatedAt(): ?\DateTime
    {
        return $this->updatedAt;
    }
    /**
     * @param \DateTime|null $updatedAt
     *
     * @return self
     */
    public function setUpdatedAt(?\DateTime $updatedAt): self
    {
        $this->initialized['updatedAt'] = true;
        $this->updatedAt = $updatedAt;
        return $this;
    }
}