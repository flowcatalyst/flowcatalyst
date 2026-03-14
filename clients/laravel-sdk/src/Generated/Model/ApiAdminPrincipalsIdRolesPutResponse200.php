<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminPrincipalsIdRolesPutResponse200 extends \ArrayObject
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
     * @var list<ApiAdminPrincipalsIdRolesPutResponse200RolesItem>|null
     */
    protected $roles;
    /**
     * @var list<string>|null
     */
    protected $added;
    /**
     * @var list<string>|null
     */
    protected $removed;
    /**
     * @return list<ApiAdminPrincipalsIdRolesPutResponse200RolesItem>|null
     */
    public function getRoles(): ?array
    {
        return $this->roles;
    }
    /**
     * @param list<ApiAdminPrincipalsIdRolesPutResponse200RolesItem>|null $roles
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
    public function getAdded(): ?array
    {
        return $this->added;
    }
    /**
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
     * @return list<string>|null
     */
    public function getRemoved(): ?array
    {
        return $this->removed;
    }
    /**
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
}