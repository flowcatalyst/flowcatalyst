<?php

namespace FlowCatalyst\Generated\Model;

class DispatchPoolFilterOptions extends \ArrayObject
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
     * @var list<FilterOption>|null
     */
    protected $dispatchPools;
    /**
     * @return list<FilterOption>|null
     */
    public function getDispatchPools(): ?array
    {
        return $this->dispatchPools;
    }
    /**
     * @param list<FilterOption>|null $dispatchPools
     *
     * @return self
     */
    public function setDispatchPools(?array $dispatchPools): self
    {
        $this->initialized['dispatchPools'] = true;
        $this->dispatchPools = $dispatchPools;
        return $this;
    }
}