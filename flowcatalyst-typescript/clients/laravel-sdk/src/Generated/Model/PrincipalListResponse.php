<?php

namespace FlowCatalyst\Generated\Model;

class PrincipalListResponse extends \ArrayObject
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
     * Principal list
     *
     * @var list<PrincipalDto>|null
     */
    protected $principals;
    /**
     * Total count
     *
     * @var int|null
     */
    protected $total;
    /**
     * Principal list
     *
     * @return list<PrincipalDto>|null
     */
    public function getPrincipals(): ?array
    {
        return $this->principals;
    }
    /**
     * Principal list
     *
     * @param list<PrincipalDto>|null $principals
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
     * Total count
     *
     * @return int|null
     */
    public function getTotal(): ?int
    {
        return $this->total;
    }
    /**
     * Total count
     *
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