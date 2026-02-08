<?php

namespace FlowCatalyst\Generated\Model;

class SyncPoolsRequest extends \ArrayObject
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
     * @var list<SyncPoolItem>|null
     */
    protected $pools;
    /**
     * @return list<SyncPoolItem>|null
     */
    public function getPools(): ?array
    {
        return $this->pools;
    }
    /**
     * @param list<SyncPoolItem>|null $pools
     *
     * @return self
     */
    public function setPools(?array $pools): self
    {
        $this->initialized['pools'] = true;
        $this->pools = $pools;
        return $this;
    }
}