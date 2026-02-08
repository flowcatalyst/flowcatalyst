<?php

namespace FlowCatalyst\Generated\Model;

class BatchEventResponse extends \ArrayObject
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
     * @var list<EventResponse>|null
     */
    protected $events;
    /**
     * @var int|null
     */
    protected $count;
    /**
     * @var int|null
     */
    protected $dispatchJobCount;
    /**
     * @var int|null
     */
    protected $duplicateCount;
    /**
     * @return list<EventResponse>|null
     */
    public function getEvents(): ?array
    {
        return $this->events;
    }
    /**
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
    /**
     * @return int|null
     */
    public function getCount(): ?int
    {
        return $this->count;
    }
    /**
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
     * @return int|null
     */
    public function getDispatchJobCount(): ?int
    {
        return $this->dispatchJobCount;
    }
    /**
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
     * @return int|null
     */
    public function getDuplicateCount(): ?int
    {
        return $this->duplicateCount;
    }
    /**
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
}