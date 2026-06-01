<?php

namespace FlowCatalyst\Generated\Model;

class PrincipalResponse extends \ArrayObject
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
     * @var bool|null
     */
    protected $active;
    /**
     * @var string|null
     */
    protected $clientId;
    /**
     * @var string|null
     */
    protected $createdAt;
    /**
     * @var string|null
     */
    protected $email;
    /**
     * Granted client IDs (matches Java's Set<String>)
     *
     * @var list<string>|null
     */
    protected $grantedClientIds;
    /**
     * @var string|null
     */
    protected $id;
    /**
     * @var string|null
     */
    protected $idpType;
    /**
     * Whether user is an anchor domain user
     *
     * @var bool|null
     */
    protected $isAnchorUser;
    /**
     * @var string|null
     */
    protected $name;
    /**
     * Role names (matches Java's Set<String>)
     *
     * @var list<string>|null
     */
    protected $roles;
    /**
     * @var string|null
     */
    protected $scope;
    /**
     * @var string|null
     */
    protected $type;
    /**
     * @var string|null
     */
    protected $updatedAt;
    /**
     * @return bool|null
     */
    public function getActive(): ?bool
    {
        return $this->active;
    }
    /**
     * @param bool|null $active
     *
     * @return self
     */
    public function setActive(?bool $active): self
    {
        $this->initialized['active'] = true;
        $this->active = $active;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getClientId(): ?string
    {
        return $this->clientId;
    }
    /**
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
     * @return string|null
     */
    public function getCreatedAt(): ?string
    {
        return $this->createdAt;
    }
    /**
     * @param string|null $createdAt
     *
     * @return self
     */
    public function setCreatedAt(?string $createdAt): self
    {
        $this->initialized['createdAt'] = true;
        $this->createdAt = $createdAt;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getEmail(): ?string
    {
        return $this->email;
    }
    /**
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
     * Granted client IDs (matches Java's Set<String>)
     *
     * @return list<string>|null
     */
    public function getGrantedClientIds(): ?array
    {
        return $this->grantedClientIds;
    }
    /**
     * Granted client IDs (matches Java's Set<String>)
     *
     * @param list<string>|null $grantedClientIds
     *
     * @return self
     */
    public function setGrantedClientIds(?array $grantedClientIds): self
    {
        $this->initialized['grantedClientIds'] = true;
        $this->grantedClientIds = $grantedClientIds;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getId(): ?string
    {
        return $this->id;
    }
    /**
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
     * @return string|null
     */
    public function getIdpType(): ?string
    {
        return $this->idpType;
    }
    /**
     * @param string|null $idpType
     *
     * @return self
     */
    public function setIdpType(?string $idpType): self
    {
        $this->initialized['idpType'] = true;
        $this->idpType = $idpType;
        return $this;
    }
    /**
     * Whether user is an anchor domain user
     *
     * @return bool|null
     */
    public function getIsAnchorUser(): ?bool
    {
        return $this->isAnchorUser;
    }
    /**
     * Whether user is an anchor domain user
     *
     * @param bool|null $isAnchorUser
     *
     * @return self
     */
    public function setIsAnchorUser(?bool $isAnchorUser): self
    {
        $this->initialized['isAnchorUser'] = true;
        $this->isAnchorUser = $isAnchorUser;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getName(): ?string
    {
        return $this->name;
    }
    /**
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
     * Role names (matches Java's Set<String>)
     *
     * @return list<string>|null
     */
    public function getRoles(): ?array
    {
        return $this->roles;
    }
    /**
     * Role names (matches Java's Set<String>)
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
     * @return string|null
     */
    public function getScope(): ?string
    {
        return $this->scope;
    }
    /**
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
    /**
     * @return string|null
     */
    public function getType(): ?string
    {
        return $this->type;
    }
    /**
     * @param string|null $type
     *
     * @return self
     */
    public function setType(?string $type): self
    {
        $this->initialized['type'] = true;
        $this->type = $type;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getUpdatedAt(): ?string
    {
        return $this->updatedAt;
    }
    /**
     * @param string|null $updatedAt
     *
     * @return self
     */
    public function setUpdatedAt(?string $updatedAt): self
    {
        $this->initialized['updatedAt'] = true;
        $this->updatedAt = $updatedAt;
        return $this;
    }
}