<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminServiceAccountsIdRolesPutResponse200 extends \ArrayObject
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
     * @var list<ApiAdminServiceAccountsIdRolesPutResponse200RolesItem>|null
     */
    protected $roles;
    /**
     * @var list<string>|null
     */
    protected $addedRoles;
    /**
     * @var list<string>|null
     */
    protected $removedRoles;
    /**
     * @return list<ApiAdminServiceAccountsIdRolesPutResponse200RolesItem>|null
     */
    public function getRoles(): ?array
    {
        return $this->roles;
    }
    /**
     * @param list<ApiAdminServiceAccountsIdRolesPutResponse200RolesItem>|null $roles
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
     * @return list<string>|null
     */
    public function getAddedRoles(): ?array
    {
        return $this->addedRoles;
    }
    /**
     * @param list<string>|null $addedRoles
     *
     * @return self
     */
    public function setAddedRoles(?array $addedRoles): self
    {
        $this->initialized['addedRoles'] = true;
        $this->addedRoles = $addedRoles;
        return $this;
    }
    /**
     * @return list<string>|null
     */
    public function getRemovedRoles(): ?array
    {
        return $this->removedRoles;
    }
    /**
     * @param list<string>|null $removedRoles
     *
     * @return self
     */
    public function setRemovedRoles(?array $removedRoles): self
    {
        $this->initialized['removedRoles'] = true;
        $this->removedRoles = $removedRoles;
        return $this;
    }
}