<?php

namespace FlowCatalyst\Generated\Model;

class ApplicationUser
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
     * A URL to the JSON Schema for this object.
     *
     * @var string|null
     */
    protected $dollarSchema;
    /**
     * @var bool|null
     */
    protected $active;
    /**
     * @var string|null
     */
    protected $clientId;
    /**
     * @var \DateTime|null
     */
    protected $createdAt;
    /**
     * @var string|null
     */
    protected $email;
    /**
     * @var string|null
     */
    protected $id;
    /**
     * @var string|null
     */
    protected $idpType;
    /**
     * @var string|null
     */
    protected $invitation;
    /**
     * @var \DateTime|null
     */
    protected $lastLoginAt;
    /**
     * @var string|null
     */
    protected $name;
    /**
     * @var list<string>|null
     */
    protected $roles;
    /**
     * A URL to the JSON Schema for this object.
     *
     * @return string|null
     */
    public function getDollarSchema(): ?string
    {
        return $this->dollarSchema;
    }
    /**
     * A URL to the JSON Schema for this object.
     *
     * @param string|null $dollarSchema
     *
     * @return self
     */
    public function setDollarSchema(?string $dollarSchema): self
    {
        $this->initialized['dollarSchema'] = true;
        $this->dollarSchema = $dollarSchema;
        return $this;
    }
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
     * @return string|null
     */
    public function getInvitation(): ?string
    {
        return $this->invitation;
    }
    /**
     * @param string|null $invitation
     *
     * @return self
     */
    public function setInvitation(?string $invitation): self
    {
        $this->initialized['invitation'] = true;
        $this->invitation = $invitation;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getLastLoginAt(): ?\DateTime
    {
        return $this->lastLoginAt;
    }
    /**
     * @param \DateTime|null $lastLoginAt
     *
     * @return self
     */
    public function setLastLoginAt(?\DateTime $lastLoginAt): self
    {
        $this->initialized['lastLoginAt'] = true;
        $this->lastLoginAt = $lastLoginAt;
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
}