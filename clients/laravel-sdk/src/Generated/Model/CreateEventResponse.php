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
     * Number of dispatch jobs created for matching subscriptions
     *
     * @var int|null
     */
    protected $dispatchJobCount;
    /**
     * Event response DTO
     *
     * @var EventResponse|null
     */
    protected $event;
    /**
     * True if this was a deduplicated request (event already existed)
     *
     * @var bool|null
     */
    protected $isDuplicate;
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
     * Event response DTO
     *
     * @return EventResponse|null
     */
    public function getEvent(): ?EventResponse
    {
        return $this->event;
    }
    /**
     * Event response DTO
     *
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
     * True if this was a deduplicated request (event already existed)
     *
     * @return bool|null
     */
    public function getIsDuplicate(): ?bool
    {
        return $this->isDuplicate;
    }
    /**
     * True if this was a deduplicated request (event already existed)
     *
     * @param bool|null $isDuplicate
     *
     * @return self
     */
    public function setIsDuplicate(?bool $isDuplicate): self
    {
        $this->initialized['isDuplicate'] = true;
        $this->isDuplicate = $isDuplicate;
        return $this;
    }
}