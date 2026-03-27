<?php

namespace FlowCatalyst\Generated\Model;

class CurrentUserResponse extends \ArrayObject
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
     * Client ID (for CLIENT scope users)
     *
     * @var string|null
     */
    protected $clientId;
    /**
     * Accessible client IDs
     *
     * @var list<string>|null
     */
    protected $clients;
    /**
     * Email address
     *
     * @var string|null
     */
    protected $email;
    /**
     * Principal ID
     *
     * @var string|null
     */
    protected $id;
    /**
     * Display name
     *
     * @var string|null
     */
    protected $name;
    /**
     * Principal type (USER, SERVICE)
     *
     * @var string|null
     */
    protected $principalType;
    /**
     * Assigned roles
     *
     * @var list<string>|null
     */
    protected $roles;
    /**
     * User scope (ANCHOR, PARTNER, CLIENT)
     *
     * @var string|null
     */
    protected $scope;
    /**
     * Client ID (for CLIENT scope users)
     *
     * @return string|null
     */
    public function getClientId(): ?string
    {
        return $this->clientId;
    }
    /**
     * Client ID (for CLIENT scope users)
     *
     * @param string|null $clientId
     *
     * @return self
     */
    public function setClientId(?string $clientId): self
    {
        $this->initialized['clientId'] = true;
        $this->clientId = $clientId;
        return $this;
    }
    /**
     * Accessible client IDs
     *
     * @return list<string>|null
     */
    public function getClients(): ?array
    {
        return $this->clients;
    }
    /**
     * Accessible client IDs
     *
     * @param list<string>|null $clients
     *
     * @return self
     */
    public function setClients(?array $clients): self
    {
        $this->initialized['clients'] = true;
        $this->clients = $clients;
        return $this;
    }
    /**
     * Email address
     *
     * @return string|null
     */
    public function getEmail(): ?string
    {
        return $this->email;
    }
    /**
     * Email address
     *
     * @param string|null $email
     *
     * @return self
     */
    public function setEmail(?string $email): self
    {
        $this->initialized['email'] = true;
        $this->email = $email;
        return $this;
    }
    /**
     * Principal ID
     *
     * @return string|null
     */
    public function getId(): ?string
    {
        return $this->id;
    }
    /**
     * Principal ID
     *
     * @param string|null $id
     *
     * @return self
     */
    public function setId(?string $id): self
    {
        $this->initialized['id'] = true;
        $this->id = $id;
        return $this;
    }
    /**
     * Display name
     *
     * @return string|null
     */
    public function getName(): ?string
    {
        return $this->name;
    }
    /**
     * Display name
     *
     * @param string|null $name
     *
     * @return self
     */
    public function setName(?string $name): self
    {
        $this->initialized['name'] = true;
        $this->name = $name;
        return $this;
    }
    /**
     * Principal type (USER, SERVICE)
     *
     * @return string|null
     */
    public function getPrincipalType(): ?string
    {
        return $this->principalType;
    }
    /**
     * Principal type (USER, SERVICE)
     *
     * @param string|null $principalType
     *
     * @return self
     */
    public function setPrincipalType(?string $principalType): self
    {
        $this->initialized['principalType'] = true;
        $this->principalType = $principalType;
        return $this;
    }
    /**
     * Assigned roles
     *
     * @return list<string>|null
     */
    public function getRoles(): ?array
    {
        return $this->roles;
    }
    /**
     * Assigned roles
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
    /**
     * User scope (ANCHOR, PARTNER, CLIENT)
     *
     * @return string|null
     */
    public function getScope(): ?string
    {
        return $this->scope;
    }
    /**
     * User scope (ANCHOR, PARTNER, CLIENT)
     *
     * @param string|null $scope
     *
     * @return self
     */
    public function setScope(?string $scope): self
    {
        $this->initialized['scope'] = true;
        $this->scope = $scope;
        return $this;
    }
}