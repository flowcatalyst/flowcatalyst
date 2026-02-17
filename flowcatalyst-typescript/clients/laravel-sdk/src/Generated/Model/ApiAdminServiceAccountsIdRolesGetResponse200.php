<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminServiceAccountsIdRolesGetResponse200 extends \ArrayObject
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
     * @var list<ApiAdminServiceAccountsIdRolesGetResponse200RolesItem>|null
     */
    protected $roles;
    /**
     * @return list<ApiAdminServiceAccountsIdRolesGetResponse200RolesItem>|null
     */
    public function getRoles(): ?array
    {
        return $this->roles;
    }
    /**
     * @param list<ApiAdminServiceAccountsIdRolesGetResponse200RolesItem>|null $roles
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