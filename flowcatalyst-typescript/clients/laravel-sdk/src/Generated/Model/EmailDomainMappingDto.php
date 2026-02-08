<?php

namespace FlowCatalyst\Generated\Model;

class EmailDomainMappingDto extends \ArrayObject
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
    protected $id;
    /**
     * @var string|null
     */
    protected $emailDomain;
    /**
     * @var string|null
     */
    protected $identityProviderId;
    /**
     * @var string|null
     */
    protected $identityProviderCode;
    /**
     * @var string|null
     */
    protected $identityProviderName;
    /**
     * @var string|null
     */
    protected $scopeType;
    /**
     * @var string|null
     */
    protected $primaryClientId;
    /**
     * @var list<string>|null
     */
    protected $additionalClientIds;
    /**
     * @var list<string>|null
     */
    protected $grantedClientIds;
    /**
     * @var list<string>|null
     */
    protected $allAccessibleClientIds;
    /**
     * @var \DateTime|null
     */
    protected $createdAt;
    /**
     * @var \DateTime|null
     */
    protected $updatedAt;
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
    public function getEmailDomain(): ?string
    {
        return $this->emailDomain;
    }
    /**
     * @param string|null $emailDomain
     *
     * @return self
     */
    public function setEmailDomain(?string $emailDomain): self
    {
        $this->initialized['emailDomain'] = true;
        $this->emailDomain = $emailDomain;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getIdentityProviderId(): ?string
    {
        return $this->identityProviderId;
    }
    /**
     * @param string|null $identityProviderId
     *
     * @return self
     */
    public function setIdentityProviderId(?string $identityProviderId): self
    {
        $this->initialized['identityProviderId'] = true;
        $this->identityProviderId = $identityProviderId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getIdentityProviderCode(): ?string
    {
        return $this->identityProviderCode;
    }
    /**
     * @param string|null $identityProviderCode
     *
     * @return self
     */
    public function setIdentityProviderCode(?string $identityProviderCode): self
    {
        $this->initialized['identityProviderCode'] = true;
        $this->identityProviderCode = $identityProviderCode;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getIdentityProviderName(): ?string
    {
        return $this->identityProviderName;
    }
    /**
     * @param string|null $identityProviderName
     *
     * @return self
     */
    public function setIdentityProviderName(?string $identityProviderName): self
    {
        $this->initialized['identityProviderName'] = true;
        $this->identityProviderName = $identityProviderName;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getScopeType(): ?string
    {
        return $this->scopeType;
    }
    /**
     * @param string|null $scopeType
     *
     * @return self
     */
    public function setScopeType(?string $scopeType): self
    {
        $this->initialized['scopeType'] = true;
        $this->scopeType = $scopeType;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getPrimaryClientId(): ?string
    {
        return $this->primaryClientId;
    }
    /**
     * @param string|null $primaryClientId
     *
     * @return self
     */
    public function setPrimaryClientId(?string $primaryClientId): self
    {
        $this->initialized['primaryClientId'] = true;
        $this->primaryClientId = $primaryClientId;
        return $this;
    }
    /**
     * @return list<string>|null
     */
    public function getAdditionalClientIds(): ?array
    {
        return $this->additionalClientIds;
    }
    /**
     * @param list<string>|null $additionalClientIds
     *
     * @return self
     */
    public function setAdditionalClientIds(?array $additionalClientIds): self
    {
        $this->initialized['additionalClientIds'] = true;
        $this->additionalClientIds = $additionalClientIds;
        return $this;
    }
    /**
     * @return list<string>|null
     */
    public function getGrantedClientIds(): ?array
    {
        return $this->grantedClientIds;
    }
    /**
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
     * @return list<string>|null
     */
    public function getAllAccessibleClientIds(): ?array
    {
        return $this->allAccessibleClientIds;
    }
    /**
     * @param list<string>|null $allAccessibleClientIds
     *
     * @return self
     */
    public function setAllAccessibleClientIds(?array $allAccessibleClientIds): self
    {
        $this->initialized['allAccessibleClientIds'] = true;
        $this->allAccessibleClientIds = $allAccessibleClientIds;
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