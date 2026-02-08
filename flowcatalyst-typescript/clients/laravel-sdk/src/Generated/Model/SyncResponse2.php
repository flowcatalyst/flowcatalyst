<?php

namespace FlowCatalyst\Generated\Model;

class SyncResponse2 extends \ArrayObject
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
     * @var int|null
     */
    protected $syncedCount;
    /**
     * @var list<RoleDto2>|null
     */
    protected $roles;
    /**
     * @return int|null
     */
    public function getSyncedCount(): ?int
    {
        return $this->syncedCount;
    }
    /**
     * @param int|null $syncedCount
     *
     * @return self
     */
    public function setSyncedCount(?int $syncedCount): self
    {
        $this->initialized['syncedCount'] = true;
        $this->syncedCount = $syncedCount;
        return $this;
    }
    /**
     * @return list<RoleDto2>|null
     */
    public function getRoles(): ?array
    {
        return $this->roles;
    }
    /**
     * @param list<RoleDto2>|null $roles
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