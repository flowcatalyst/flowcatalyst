<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminRolesBySourceSourceGetResponse200 extends \ArrayObject
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
     * @var list<ApiAdminRolesBySourceSourceGetResponse200RolesItem>|null
     */
    protected $roles;
    /**
     * @return list<ApiAdminRolesBySourceSourceGetResponse200RolesItem>|null
     */
    public function getRoles(): ?array
    {
        return $this->roles;
    }
    /**
     * @param list<ApiAdminRolesBySourceSourceGetResponse200RolesItem>|null $roles
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