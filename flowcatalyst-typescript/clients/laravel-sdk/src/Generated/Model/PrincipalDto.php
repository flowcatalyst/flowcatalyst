<?php

namespace FlowCatalyst\Generated\Model;

class PrincipalDto extends \ArrayObject
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
     * Principal ID
     *
     * @var string|null
     */
    protected $id;
    /**
     * Principal type (USER/SERVICE)
     *
     * @var string|null
     */
    protected $type;
    /**
     * User scope (ANCHOR/PARTNER/CLIENT)
     *
     * @var string|null
     */
    protected $scope;
    /**
     * Home client ID
     *
     * @var string|null
     */
    protected $clientId;
    /**
     * Display name
     *
     * @var string|null
     */
    protected $name;
    /**
     * Whether principal is active
     *
     * @var bool|null
     */
    protected $active;
    /**
     * Email address (USER type only)
     *
     * @var string|null
     */
    protected $email;
    /**
     * Identity provider type
     *
     * @var string|null
     */
    protected $idpType;
    /**
     * Assigned role names
     *
     * @var list<string>|null
     */
    protected $roles;
    /**
     * Whether user is from anchor domain
     *
     * @var bool|null
     */
    protected $isAnchorUser;
    /**
     * Granted client IDs
     *
     * @var list<string>|null
     */
    protected $grantedClientIds;
    /**
     * @var \DateTime|null
     */
    protected $createdAt;
    /**
     * @var \DateTime|null
     */
    protected $updatedAt;
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
     * Principal type (USER/SERVICE)
     *
     * @return string|null
     */
    public function getType(): ?string
    {
        return $this->type;
    }
    /**
     * Principal type (USER/SERVICE)
     *
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
     * User scope (ANCHOR/PARTNER/CLIENT)
     *
     * @return string|null
     */
    public function getScope(): ?string
    {
        return $this->scope;
    }
    /**
     * User scope (ANCHOR/PARTNER/CLIENT)
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
    /**
     * Home client ID
     *
     * @return string|null
     */
    public function getClientId(): ?string
    {
        return $this->clientId;
    }
    /**
     * Home client ID
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
     * Whether principal is active
     *
     * @return bool|null
     */
    public function getActive(): ?bool
    {
        return $this->active;
    }
    /**
     * Whether principal is active
     *
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
     * Email address (USER type only)
     *
     * @return string|null
     */
    public function getEmail(): ?string
    {
        return $this->email;
    }
    /**
     * Email address (USER type only)
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
     * Identity provider type
     *
     * @return string|null
     */
    public function getIdpType(): ?string
    {
        return $this->idpType;
    }
    /**
     * Identity provider type
     *
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
     * Assigned role names
     *
     * @return list<string>|null
     */
    public function getRoles(): ?array
    {
        return $this->roles;
    }
    /**
     * Assigned role names
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
     * Whether user is from anchor domain
     *
     * @return bool|null
     */
    public function getIsAnchorUser(): ?bool
    {
        return $this->isAnchorUser;
    }
    /**
     * Whether user is from anchor domain
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
     * Granted client IDs
     *
     * @return list<string>|null
     */
    public function getGrantedClientIds(): ?array
    {
        return $this->grantedClientIds;
    }
    /**
     * Granted client IDs
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
     * @return \DateTime|null
     */
    public function getCreatedAt(): ?\DateTime
    {
        return $this->createdAt;
    }
    /**
     * @param \DateTime|null $createdAt
     *
     * @return self
     */
    public function setCreatedAt(?\DateTime $createdAt): self
    {
        $this->initialized['createdAt'] = true;
        $this->createdAt = $createdAt;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getUpdatedAt(): ?\DateTime
    {
        return $this->updatedAt;
    }
    /**
     * @param \DateTime|null $updatedAt
     *
     * @return self
     */
    public function setUpdatedAt(?\DateTime $updatedAt): self
    {
        $this->initialized['updatedAt'] = true;
        $this->updatedAt = $updatedAt;
        return $this;
    }
}