<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminSubscriptionsIdPutBody extends \ArrayObject
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
    protected $name;
    /**
     * @var mixed|null
     */
    protected $description;
    /**
     * @var list<ApiAdminSubscriptionsIdPutBodyEventTypesItem>|null
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
     * @var list<ApiAdminSubscriptionsIdPutBodyCustomConfigItem>|null
     */
    protected $customConfig;
    /**
     * @var mixed|null
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
     * @var mixed|null
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
     * @return list<ApiAdminSubscriptionsIdPutBodyEventTypesItem>|null
     */
    public function getEventTypes(): ?array
    {
        return $this->eventTypes;
    }
    /**
     * @param list<ApiAdminSubscriptionsIdPutBodyEventTypesItem>|null $eventTypes
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
     * @return list<ApiAdminSubscriptionsIdPutBodyCustomConfigItem>|null
     */
    public function getCustomConfig(): ?array
    {
        return $this->customConfig;
    }
    /**
     * @param list<ApiAdminSubscriptionsIdPutBodyCustomConfigItem>|null $customConfig
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
     * @return mixed
     */
    public function getStatus()
    {
        return $this->status;
    }
    /**
     * @param mixed $status
     *
     * @return self
     */
    public function setStatus($status): self
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
     * @return mixed
     */
    public function getMode()
    {
        return $this->mode;
    }
    /**
     * @param mixed $mode
     *
     * @return self
     */
    public function setMode($mode): self
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
}