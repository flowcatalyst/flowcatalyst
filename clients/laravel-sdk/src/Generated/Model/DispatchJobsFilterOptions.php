<?php

namespace FlowCatalyst\Generated\Model;

class DispatchJobsFilterOptions extends \ArrayObject
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
     * @var list<FilterOption>|null
     */
    protected $clients;
    /**
     * @var list<FilterOption>|null
     */
    protected $eventTypes;
    /**
     * @var list<FilterOption>|null
     */
    protected $statuses;
    /**
     * @var list<FilterOption>|null
     */
    protected $subscriptions;
    /**
     * @return list<FilterOption>|null
     */
    public function getClients(): ?array
    {
        return $this->clients;
    }
    /**
     * @param list<FilterOption>|null $clients
     *
     * @return self
     */
    public function setClients(?array $clients): self
    {
        $this->initialized['clients'] = true;
        $this->clients = $clients;
        return $this;
    }
    /**
     * @return list<FilterOption>|null
     */
    public function getEventTypes(): ?array
    {
        return $this->eventTypes;
    }
    /**
     * @param list<FilterOption>|null $eventTypes
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
     * @return list<FilterOption>|null
     */
    public function getStatuses(): ?array
    {
        return $this->statuses;
    }
    /**
     * @param list<FilterOption>|null $statuses
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
     * @return list<FilterOption>|null
     */
    public function getSubscriptions(): ?array
    {
        return $this->subscriptions;
    }
    /**
     * @param list<FilterOption>|null $subscriptions
     *
     * @return self
     */
    public function setSubscriptions(?array $subscriptions): self
    {
        $this->initialized['subscriptions'] = true;
        $this->subscriptions = $subscriptions;
        return $this;
    }
}