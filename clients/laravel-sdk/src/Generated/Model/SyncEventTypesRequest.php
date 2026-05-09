<?php

namespace FlowCatalyst\Generated\Model;

class SyncEventTypesRequest extends \ArrayObject
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
     * Application code
     *
     * @var string|null
     */
    protected $applicationCode;
    /**
     * Event types to sync
     *
     * @var list<SyncEventTypeInputRequest>|null
     */
    protected $eventTypes;
    /**
     * Application code
     *
     * @return string|null
     */
    public function getApplicationCode(): ?string
    {
        return $this->applicationCode;
    }
    /**
     * Application code
     *
     * @param string|null $applicationCode
     *
     * @return self
     */
    public function setApplicationCode(?string $applicationCode): self
    {
        $this->initialized['applicationCode'] = true;
        $this->applicationCode = $applicationCode;
        return $this;
    }
    /**
     * Event types to sync
     *
     * @return list<SyncEventTypeInputRequest>|null
     */
    public function getEventTypes(): ?array
    {
        return $this->eventTypes;
    }
    /**
     * Event types to sync
     *
     * @param list<SyncEventTypeInputRequest>|null $eventTypes
     *
     * @return self
     */
    public function setEventTypes(?array $eventTypes): self
    {
        $this->initialized['eventTypes'] = true;
        $this->eventTypes = $eventTypes;
        return $this;
    }
}