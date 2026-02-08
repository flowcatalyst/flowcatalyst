<?php

namespace FlowCatalyst\Generated\Model;

class RoleListResponse extends \ArrayObject
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
     * Role assignments
     *
     * @var list<RoleAssignmentDto>|null
     */
    protected $roles;
    /**
     * Role assignments
     *
     * @return list<RoleAssignmentDto>|null
     */
    public function getRoles(): ?array
    {
        return $this->roles;
    }
    /**
     * Role assignments
     *
     * @param list<RoleAssignmentDto>|null $roles
     *
     * @return self
     */
    public function setRoles(?array $roles): self
    {
        $this->initialized['roles'] = true;
        $this->roles = $roles;
        return $this;
    }
}