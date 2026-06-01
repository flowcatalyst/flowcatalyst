<?php

namespace FlowCatalyst\Generated\Model;

class PermissionListResponse extends \ArrayObject
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
     * @var list<PermissionResponse>|null
     */
    protected $permissions;
    /**
     * @var int|null
     */
    protected $total;
    /**
     * @return list<PermissionResponse>|null
     */
    public function getPermissions(): ?array
    {
        return $this->permissions;
    }
    /**
     * @param list<PermissionResponse>|null $permissions
     *
     * @return self
     */
    public function setPermissions(?array $permissions): self
    {
        $this->initialized['permissions'] = true;
        $this->permissions = $permissions;
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