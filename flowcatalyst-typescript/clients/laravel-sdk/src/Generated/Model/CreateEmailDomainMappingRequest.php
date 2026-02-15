<?php

namespace FlowCatalyst\Generated\Model;

class CreateEmailDomainMappingRequest extends \ArrayObject
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
    protected $emailDomain;
    /**
     * @var string|null
     */
    protected $identityProviderId;
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
     * @var string|null
     */
    protected $requiredOidcTenantId;
    /**
     * @var list<string>|null
     */
    protected $allowedRoleIds;
    /**
     * @var bool|null
     */
    protected $syncRolesFromIdp;
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
     * @return string|null
     */
    public function getRequiredOidcTenantId(): ?string
    {
        return $this->requiredOidcTenantId;
    }
    /**
     * @param string|null $requiredOidcTenantId
     *
     * @return self
     */
    public function setRequiredOidcTenantId(?string $requiredOidcTenantId): self
    {
        $this->initialized['requiredOidcTenantId'] = true;
        $this->requiredOidcTenantId = $requiredOidcTenantId;
        return $this;
    }
    /**
     * @return list<string>|null
     */
    public function getAllowedRoleIds(): ?array
    {
        return $this->allowedRoleIds;
    }
    /**
     * @param list<string>|null $allowedRoleIds
     *
     * @return self
     */
    public function setAllowedRoleIds(?array $allowedRoleIds): self
    {
        $this->initialized['allowedRoleIds'] = true;
        $this->allowedRoleIds = $allowedRoleIds;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getSyncRolesFromIdp(): ?bool
    {
        return $this->syncRolesFromIdp;
    }
    /**
     * @param bool|null $syncRolesFromIdp
     *
     * @return self
     */
    public function setSyncRolesFromIdp(?bool $syncRolesFromIdp): self
    {
        $this->initialized['syncRolesFromIdp'] = true;
        $this->syncRolesFromIdp = $syncRolesFromIdp;
        return $this;
    }
}