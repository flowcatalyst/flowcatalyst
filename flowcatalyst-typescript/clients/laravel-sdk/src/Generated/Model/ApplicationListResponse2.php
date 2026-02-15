<?php

namespace FlowCatalyst\Generated\Model;

class ApplicationListResponse2 extends \ArrayObject
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
     * @var list<ApplicationResponse2>|null
     */
    protected $items;
    /**
     * @return list<ApplicationResponse2>|null
     */
    public function getItems(): ?array
    {
        return $this->items;
    }
    /**
     * @param list<ApplicationResponse2>|null $items
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