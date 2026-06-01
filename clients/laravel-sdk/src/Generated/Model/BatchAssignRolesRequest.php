<?php

namespace FlowCatalyst\Generated\Model;

class BatchAssignRolesRequest extends \ArrayObject
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
     * List of role codes to assign (replaces existing roles)
     *
     * @var list<string>|null
     */
    protected $roles;
    /**
     * List of role codes to assign (replaces existing roles)
     *
     * @return list<string>|null
     */
    public function getRoles(): ?array
    {
        return $this->roles;
    }
    /**
     * List of role codes to assign (replaces existing roles)
     *
     * @param list<string>|null $roles
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