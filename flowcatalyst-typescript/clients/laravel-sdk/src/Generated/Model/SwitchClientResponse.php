<?php

namespace FlowCatalyst\Generated\Model;

class SwitchClientResponse extends \ArrayObject
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
     * @var string|null
     */
    protected $token;
    /**
     * @var ClientInfo|null
     */
    protected $client;
    /**
     * @var list<string>|null
     */
    protected $roles;
    /**
     * @var list<string>|null
     */
    protected $permissions;
    /**
     * @return string|null
     */
    public function getToken(): ?string
    {
        return $this->token;
    }
    /**
     * @param string|null $token
     *
     * @return self
     */
    public function setToken(?string $token): self
    {
        $this->initialized['token'] = true;
        $this->token = $token;
        return $this;
    }
    /**
     * @return ClientInfo|null
     */
    public function getClient(): ?ClientInfo
    {
        return $this->client;
    }
    /**
     * @param ClientInfo|null $client
     *
     * @return self
     */
    public function setClient(?ClientInfo $client): self
    {
        $this->initialized['client'] = true;
        $this->client = $client;
        return $this;
    }
    /**
     * @return list<string>|null
     */
    public function getRoles(): ?array
    {
        return $this->roles;
    }
    /**
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
    /**
     * @return list<string>|null
     */
    public function getPermissions(): ?array
    {
        return $this->permissions;
    }
    /**
     * @param list<string>|null $permissions
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