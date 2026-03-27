<?php

namespace FlowCatalyst\Generated\Model;

class BatchCreateEventsRequest extends \ArrayObject
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
     * @var list<CreateEventRequest>|null
     */
    protected $events;
    /**
     * @return list<CreateEventRequest>|null
     */
    public function getEvents(): ?array
    {
        return $this->events;
    }
    /**
     * @param list<CreateEventRequest>|null $events
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