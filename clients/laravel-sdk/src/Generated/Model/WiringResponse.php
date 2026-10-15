<?php

namespace FlowCatalyst\Generated\Model;

class WiringResponse
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
     * A URL to the JSON Schema for this object.
     *
     * @var string|null
     */
    protected $dollarSchema;
    /**
     * @var string|null
     */
    protected $dispatchPoolCode;
    /**
     * @var int|null
     */
    protected $schedulesCreated;
    /**
     * @var int|null
     */
    protected $schedulesDeleted;
    /**
     * @var int|null
     */
    protected $schedulesUpdated;
    /**
     * @var int|null
     */
    protected $subscriptionsCreated;
    /**
     * @var int|null
     */
    protected $subscriptionsDeleted;
    /**
     * @var int|null
     */
    protected $subscriptionsUpdated;
    /**
     * A URL to the JSON Schema for this object.
     *
     * @return string|null
     */
    public function getDollarSchema(): ?string
    {
        return $this->dollarSchema;
    }
    /**
     * A URL to the JSON Schema for this object.
     *
     * @param string|null $dollarSchema
     *
     * @return self
     */
    public function setDollarSchema(?string $dollarSchema): self
    {
        $this->initialized['dollarSchema'] = true;
        $this->dollarSchema = $dollarSchema;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getDispatchPoolCode(): ?string
    {
        return $this->dispatchPoolCode;
    }
    /**
     * @param string|null $dispatchPoolCode
     *
     * @return self
     */
    public function setDispatchPoolCode(?string $dispatchPoolCode): self
    {
        $this->initialized['dispatchPoolCode'] = true;
        $this->dispatchPoolCode = $dispatchPoolCode;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getSchedulesCreated(): ?int
    {
        return $this->schedulesCreated;
    }
    /**
     * @param int|null $schedulesCreated
     *
     * @return self
     */
    public function setSchedulesCreated(?int $schedulesCreated): self
    {
        $this->initialized['schedulesCreated'] = true;
        $this->schedulesCreated = $schedulesCreated;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getSchedulesDeleted(): ?int
    {
        return $this->schedulesDeleted;
    }
    /**
     * @param int|null $schedulesDeleted
     *
     * @return self
     */
    public function setSchedulesDeleted(?int $schedulesDeleted): self
    {
        $this->initialized['schedulesDeleted'] = true;
        $this->schedulesDeleted = $schedulesDeleted;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getSchedulesUpdated(): ?int
    {
        return $this->schedulesUpdated;
    }
    /**
     * @param int|null $schedulesUpdated
     *
     * @return self
     */
    public function setSchedulesUpdated(?int $schedulesUpdated): self
    {
        $this->initialized['schedulesUpdated'] = true;
        $this->schedulesUpdated = $schedulesUpdated;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getSubscriptionsCreated(): ?int
    {
        return $this->subscriptionsCreated;
    }
    /**
     * @param int|null $subscriptionsCreated
     *
     * @return self
     */
    public function setSubscriptionsCreated(?int $subscriptionsCreated): self
    {
        $this->initialized['subscriptionsCreated'] = true;
        $this->subscriptionsCreated = $subscriptionsCreated;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getSubscriptionsDeleted(): ?int
    {
        return $this->subscriptionsDeleted;
    }
    /**
     * @param int|null $subscriptionsDeleted
     *
     * @return self
     */
    public function setSubscriptionsDeleted(?int $subscriptionsDeleted): self
    {
        $this->initialized['subscriptionsDeleted'] = true;
        $this->subscriptionsDeleted = $subscriptionsDeleted;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getSubscriptionsUpdated(): ?int
    {
        return $this->subscriptionsUpdated;
    }
    /**
     * @param int|null $subscriptionsUpdated
     *
     * @return self
     */
    public function setSubscriptionsUpdated(?int $subscriptionsUpdated): self
    {
        $this->initialized['subscriptionsUpdated'] = true;
        $this->subscriptionsUpdated = $subscriptionsUpdated;
        return $this;
    }
}