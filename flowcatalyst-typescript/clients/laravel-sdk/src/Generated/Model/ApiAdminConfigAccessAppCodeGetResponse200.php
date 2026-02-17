<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminConfigAccessAppCodeGetResponse200 extends \ArrayObject
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
     * @var list<ApiAdminConfigAccessAppCodeGetResponse200ItemsItem>|null
     */
    protected $items;
    /**
     * @return list<ApiAdminConfigAccessAppCodeGetResponse200ItemsItem>|null
     */
    public function getItems(): ?array
    {
        return $this->items;
    }
    /**
     * @param list<ApiAdminConfigAccessAppCodeGetResponse200ItemsItem>|null $items
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