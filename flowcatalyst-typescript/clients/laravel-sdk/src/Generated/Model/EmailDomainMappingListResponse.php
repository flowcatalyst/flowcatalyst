<?php

namespace FlowCatalyst\Generated\Model;

class EmailDomainMappingListResponse extends \ArrayObject
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
     * @var list<EmailDomainMappingDto>|null
     */
    protected $mappings;
    /**
     * @var int|null
     */
    protected $total;
    /**
     * @return list<EmailDomainMappingDto>|null
     */
    public function getMappings(): ?array
    {
        return $this->mappings;
    }
    /**
     * @param list<EmailDomainMappingDto>|null $mappings
     *
     * @return self
     */
    public function setMappings(?array $mappings): self
    {
        $this->initialized['mappings'] = true;
        $this->mappings = $mappings;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getTotal(): ?int
    {
        return $this->total;
    }
    /**
     * @param int|null $total
     *
     * @return self
     */
    public function setTotal(?int $total): self
    {
        $this->initialized['total'] = true;
        $this->total = $total;
        return $this;
    }
}