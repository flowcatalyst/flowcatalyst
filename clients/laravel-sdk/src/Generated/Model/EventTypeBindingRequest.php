<?php

namespace FlowCatalyst\Generated\Model;

class EventTypeBindingRequest extends \ArrayObject
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
     * Event type code (with optional wildcards)
     *
     * @var string|null
     */
    protected $eventTypeCode;
    /**
     * Optional filter expression
     *
     * @var string|null
     */
    protected $filter;
    /**
     * Event type code (with optional wildcards)
     *
     * @return string|null
     */
    public function getEventTypeCode(): ?string
    {
        return $this->eventTypeCode;
    }
    /**
     * Event type code (with optional wildcards)
     *
     * @param string|null $eventTypeCode
     *
     * @return self
     */
    public function setEventTypeCode(?string $eventTypeCode): self
    {
        $this->initialized['eventTypeCode'] = true;
        $this->eventTypeCode = $eventTypeCode;
        return $this;
    }
    /**
     * Optional filter expression
     *
     * @return string|null
     */
    public function getFilter(): ?string
    {
        return $this->filter;
    }
    /**
     * Optional filter expression
     *
     * @param string|null $filter
     *
     * @return self
     */
    public function setFilter(?string $filter): self
    {
        $this->initialized['filter'] = true;
        $this->filter = $filter;
        return $this;
    }
}