<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminRolesPermissionsGetResponse200 extends \ArrayObject
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
     * @var list<ApiAdminRolesPermissionsGetResponse200PermissionsItem>|null
     */
    protected $permissions;
    /**
     * @return list<ApiAdminRolesPermissionsGetResponse200PermissionsItem>|null
     */
    public function getPermissions(): ?array
    {
        return $this->permissions;
    }
    /**
     * @param list<ApiAdminRolesPermissionsGetResponse200PermissionsItem>|null $permissions
     *
     * @return self
     */
    public function setPermissions(?array $permissions): self
    {
        $this->initialized['permissions'] = true;
        $this->permissions = $permissions;
        return $this;
    }
}