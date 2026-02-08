<?php

namespace FlowCatalyst\Generated\Model;

class ClientListResponse3 extends \ArrayObject
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
     * @var list<ClientResponse>|null
     */
    protected $items;
    /**
     * @return list<ClientResponse>|null
     */
    public function getItems(): ?array
    {
        return $this->items;
    }
    /**
     * @param list<ClientResponse>|null $items
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