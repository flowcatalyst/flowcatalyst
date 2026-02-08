<?php

namespace FlowCatalyst\Generated\Model;

class RoleListResponse1 extends \ArrayObject
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
     * Role list
     *
     * @var list<RoleDto>|null
     */
    protected $roles;
    /**
     * Total count
     *
     * @var int|null
     */
    protected $total;
    /**
     * Role list
     *
     * @return list<RoleDto>|null
     */
    public function getRoles(): ?array
    {
        return $this->roles;
    }
    /**
     * Role list
     *
     * @param list<RoleDto>|null $roles
     *
     * @return self
     */
    public function setRoles(?array $roles): self
    {
        $this->initialized['roles'] = true;
        $this->roles = $roles;
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