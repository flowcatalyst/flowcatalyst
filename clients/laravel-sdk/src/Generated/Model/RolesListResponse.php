<?php

namespace FlowCatalyst\Generated\Model;

class RolesListResponse extends \ArrayObject
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
     * @var list<RoleAssignmentDto>|null
     */
    protected $roles;
    /**
     * @return list<RoleAssignmentDto>|null
     */
    public function getRoles(): ?array
    {
        return $this->roles;
    }
    /**
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