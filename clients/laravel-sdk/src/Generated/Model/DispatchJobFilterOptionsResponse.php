<?php

namespace FlowCatalyst\Generated\Model;

class DispatchJobFilterOptionsResponse extends \ArrayObject
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
     * @var list<string>|null
     */
    protected $eventTypeCodes;
    /**
     * @var list<string>|null
     */
    protected $modes;
    /**
     * @var list<string>|null
     */
    protected $statuses;
    /**
     * @var list<string>|null
     */
    protected $subscriptionIds;
    /**
     * @return list<string>|null
     */
    public function getEventTypeCodes(): ?array
    {
        return $this->eventTypeCodes;
    }
    /**
     * @param list<string>|null $eventTypeCodes
     *
     * @return self
     */
    public function setEventTypeCodes(?array $eventTypeCodes): self
    {
        $this->initialized['eventTypeCodes'] = true;
        $this->eventTypeCodes = $eventTypeCodes;
        return $this;
    }
    /**
     * @return list<string>|null
     */
    public function getModes(): ?array
    {
        return $this->modes;
    }
    /**
     * @param list<string>|null $modes
     *
     * @return self
     */
    public function setModes(?array $modes): self
    {
        $this->initialized['modes'] = true;
        $this->modes = $modes;
        return $this;
    }
    /**
     * @return list<string>|null
     */
    public function getStatuses(): ?array
    {
        return $this->statuses;
    }
    /**
     * @param list<string>|null $statuses
     *
     * @return self
     */
    public function setStatuses(?array $statuses): self
    {
        $this->initialized['statuses'] = true;
        $this->statuses = $statuses;
        return $this;
    }
    /**
     * @return list<string>|null
     */
    public function getSubscriptionIds(): ?array
    {
        return $this->subscriptionIds;
    }
    /**
     * @param list<string>|null $subscriptionIds
     *
     * @return self
     */
    public function setSubscriptionIds(?array $subscriptionIds): self
    {
        $this->initialized['subscriptionIds'] = true;
        $this->subscriptionIds = $subscriptionIds;
        return $this;
    }
}