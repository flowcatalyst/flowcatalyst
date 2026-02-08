<?php

namespace FlowCatalyst\Generated\Model;

class SyncResponse1 extends \ArrayObject
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
     * @var int|null
     */
    protected $created;
    /**
     * @var int|null
     */
    protected $updated;
    /**
     * @var int|null
     */
    protected $deleted;
    /**
     * @var list<EventTypeDto>|null
     */
    protected $eventTypes;
    /**
     * @return int|null
     */
    public function getCreated(): ?int
    {
        return $this->created;
    }
    /**
     * @param int|null $created
     *
     * @return self
     */
    public function setCreated(?int $created): self
    {
        $this->initialized['created'] = true;
        $this->created = $created;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getUpdated(): ?int
    {
        return $this->updated;
    }
    /**
     * @param int|null $updated
     *
     * @return self
     */
    public function setUpdated(?int $updated): self
    {
        $this->initialized['updated'] = true;
        $this->updated = $updated;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getDeleted(): ?int
    {
        return $this->deleted;
    }
    /**
     * @param int|null $deleted
     *
     * @return self
     */
    public function setDeleted(?int $deleted): self
    {
        $this->initialized['deleted'] = true;
        $this->deleted = $deleted;
        return $this;
    }
    /**
     * @return list<EventTypeDto>|null
     */
    public function getEventTypes(): ?array
    {
        return $this->eventTypes;
    }
    /**
     * @param list<EventTypeDto>|null $eventTypes
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