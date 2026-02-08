<?php

namespace FlowCatalyst\Generated\Model;

class SyncResponse extends \ArrayObject
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
     * @var list<PoolDto>|null
     */
    protected $pools;
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
     * @return list<PoolDto>|null
     */
    public function getPools(): ?array
    {
        return $this->pools;
    }
    /**
     * @param list<PoolDto>|null $pools
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