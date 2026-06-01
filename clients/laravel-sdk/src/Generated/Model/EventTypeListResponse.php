<?php

namespace FlowCatalyst\Generated\Model;

class EventTypeListResponse extends \ArrayObject
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
     * @var list<EventTypeResponse>|null
     */
    protected $items;
    /**
     * @return list<EventTypeResponse>|null
     */
    public function getItems(): ?array
    {
        return $this->items;
    }
    /**
     * @param list<EventTypeResponse>|null $items
     *
     * @return self
     */
    public function setItems(?array $items): self
    {
        $this->initialized['items'] = true;
        $this->items = $items;
        return $this;
    }
}