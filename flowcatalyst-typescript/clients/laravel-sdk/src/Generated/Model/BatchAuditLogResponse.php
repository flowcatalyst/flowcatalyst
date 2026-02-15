<?php

namespace FlowCatalyst\Generated\Model;

class BatchAuditLogResponse extends \ArrayObject
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
    protected $count;
    /**
     * @return int|null
     */
    public function getCount(): ?int
    {
        return $this->count;
    }
    /**
     * @param int|null $count
     *
     * @return self
     */
    public function setCount(?int $count): self
    {
        $this->initialized['count'] = true;
        $this->count = $count;
        return $this;
    }
}