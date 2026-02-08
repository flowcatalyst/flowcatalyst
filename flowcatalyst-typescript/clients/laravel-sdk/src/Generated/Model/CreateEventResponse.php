<?php

namespace FlowCatalyst\Generated\Model;

class CreateEventResponse extends \ArrayObject
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
     * @var EventResponse|null
     */
    protected $event;
    /**
     * @var int|null
     */
    protected $dispatchJobCount;
    /**
     * @return EventResponse|null
     */
    public function getEvent(): ?EventResponse
    {
        return $this->event;
    }
    /**
     * @param EventResponse|null $event
     *
     * @return self
     */
    public function setEvent(?EventResponse $event): self
    {
        $this->initialized['event'] = true;
        $this->event = $event;
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
}