<?php

namespace FlowCatalyst\Generated\Model;

class ApiSdkPrincipalsGetResponse200 extends \ArrayObject
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
     * @var list<ApiSdkPrincipalsGetResponse200PrincipalsItem>|null
     */
    protected $principals;
    /**
     * @var int|null
     */
    protected $total;
    /**
     * @return list<ApiSdkPrincipalsGetResponse200PrincipalsItem>|null
     */
    public function getPrincipals(): ?array
    {
        return $this->principals;
    }
    /**
     * @param list<ApiSdkPrincipalsGetResponse200PrincipalsItem>|null $principals
     *
     * @return self
     */
    public function setPrincipals(?array $principals): self
    {
        $this->initialized['principals'] = true;
        $this->principals = $principals;
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