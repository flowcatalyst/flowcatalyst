<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminEventTypesGetResponse200 extends \ArrayObject
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
     * @var list<ApiAdminEventTypesGetResponse200EventTypesItem>|null
     */
    protected $eventTypes;
    /**
     * @var int|null
     */
    protected $total;
    /**
     * @return list<ApiAdminEventTypesGetResponse200EventTypesItem>|null
     */
    public function getEventTypes(): ?array
    {
        return $this->eventTypes;
    }
    /**
     * @param list<ApiAdminEventTypesGetResponse200EventTypesItem>|null $eventTypes
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
     * @return int|null
     */
    public function getTotal(): ?int
    {
        return $this->total;
    }
    /**
     * @param int|null $total
     *
     * @return self
     */
    public function setTotal(?int $total): self
    {
        $this->initialized['total'] = true;
        $this->total = $total;
        return $this;
    }
}