<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminAnchorDomainsGetResponse200 extends \ArrayObject
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
     * @var list<ApiAdminAnchorDomainsGetResponse200AnchorDomainsItem>|null
     */
    protected $anchorDomains;
    /**
     * @var int|null
     */
    protected $total;
    /**
     * @return list<ApiAdminAnchorDomainsGetResponse200AnchorDomainsItem>|null
     */
    public function getAnchorDomains(): ?array
    {
        return $this->anchorDomains;
    }
    /**
     * @param list<ApiAdminAnchorDomainsGetResponse200AnchorDomainsItem>|null $anchorDomains
     *
     * @return self
     */
    public function setAnchorDomains(?array $anchorDomains): self
    {
        $this->initialized['anchorDomains'] = true;
        $this->anchorDomains = $anchorDomains;
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