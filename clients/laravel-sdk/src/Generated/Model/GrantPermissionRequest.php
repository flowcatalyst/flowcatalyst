<?php

namespace FlowCatalyst\Generated\Model;

class GrantPermissionRequest extends \ArrayObject
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
     * Permission to grant
     *
     * @var string|null
     */
    protected $permission;
    /**
     * Permission to grant
     *
     * @return string|null
     */
    public function getPermission(): ?string
    {
        return $this->permission;
    }
    /**
     * Permission to grant
     *
     * @param string|null $permission
     *
     * @return self
     */
    public function setPermission(?string $permission): self
    {
        $this->initialized['permission'] = true;
        $this->permission = $permission;
        return $this;
    }
}