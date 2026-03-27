<?php

namespace FlowCatalyst\Generated\Model;

class BatchCreateResponse extends \ArrayObject
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
     * Total number of events in response
     *
     * @var int|null
     */
    protected $count;
    /**
     * Number of dispatch jobs created for matching subscriptions
     *
     * @var int|null
     */
    protected $dispatchJobCount;
    /**
     * Number of events that were deduplicated (already existed)
     *
     * @var int|null
     */
    protected $duplicateCount;
    /**
     * All created events (new and deduplicated)
     *
     * @var list<EventResponse>|null
     */
    protected $events;
    /**
     * Total number of events in response
     *
     * @return int|null
     */
    public function getCount(): ?int
    {
        return $this->count;
    }
    /**
     * Total number of events in response
     *
     * @param int|null $count
     *
     * @return self
     */
    public function setCount(?int $count): self
    {
        $this->initialized['count'] = true;
        $this->count = $count;
        return $this;
    }
    /**
     * Number of dispatch jobs created for matching subscriptions
     *
     * @return int|null
     */
    public function getDispatchJobCount(): ?int
    {
        return $this->dispatchJobCount;
    }
    /**
     * Number of dispatch jobs created for matching subscriptions
     *
     * @param int|null $dispatchJobCount
     *
     * @return self
     */
    public function setDispatchJobCount(?int $dispatchJobCount): self
    {
        $this->initialized['dispatchJobCount'] = true;
        $this->dispatchJobCount = $dispatchJobCount;
        return $this;
    }
    /**
     * Number of events that were deduplicated (already existed)
     *
     * @return int|null
     */
    public function getDuplicateCount(): ?int
    {
        return $this->duplicateCount;
    }
    /**
     * Number of events that were deduplicated (already existed)
     *
     * @param int|null $duplicateCount
     *
     * @return self
     */
    public function setDuplicateCount(?int $duplicateCount): self
    {
        $this->initialized['duplicateCount'] = true;
        $this->duplicateCount = $duplicateCount;
        return $this;
    }
    /**
     * All created events (new and deduplicated)
     *
     * @return list<EventResponse>|null
     */
    public function getEvents(): ?array
    {
        return $this->events;
    }
    /**
     * All created events (new and deduplicated)
     *
     * @param list<EventResponse>|null $events
     *
     * @return self
     */
    public function setEvents(?array $events): self
    {
        $this->initialized['events'] = true;
        $this->events = $events;
        return $this;
    }
}