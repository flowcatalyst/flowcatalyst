<?php

namespace FlowCatalyst\Generated\Model;

class BatchAssignRolesResponse extends \ArrayObject
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
     * Roles that were added
     *
     * @var list<string>|null
     */
    protected $added;
    /**
     * Roles that were removed
     *
     * @var list<string>|null
     */
    protected $removed;
    /**
     * Current role assignments after update
     *
     * @var list<RoleAssignmentDto>|null
     */
    protected $roles;
    /**
     * Roles that were added
     *
     * @return list<string>|null
     */
    public function getAdded(): ?array
    {
        return $this->added;
    }
    /**
     * Roles that were added
     *
     * @param list<string>|null $added
     *
     * @return self
     */
    public function setAdded(?array $added): self
    {
        $this->initialized['added'] = true;
        $this->added = $added;
        return $this;
    }
    /**
     * Roles that were removed
     *
     * @return list<string>|null
     */
    public function getRemoved(): ?array
    {
        return $this->removed;
    }
    /**
     * Roles that were removed
     *
     * @param list<string>|null $removed
     *
     * @return self
     */
    public function setRemoved(?array $removed): self
    {
        $this->initialized['removed'] = true;
        $this->removed = $removed;
        return $this;
    }
    /**
     * Current role assignments after update
     *
     * @return list<RoleAssignmentDto>|null
     */
    public function getRoles(): ?array
    {
        return $this->roles;
    }
    /**
     * Current role assignments after update
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