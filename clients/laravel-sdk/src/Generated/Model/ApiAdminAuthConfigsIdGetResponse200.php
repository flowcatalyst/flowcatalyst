<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminAuthConfigsIdGetResponse200 extends \ArrayObject
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
    protected $configType;
    /**
     * @var mixed|null
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
    protected $authProvider;
    /**
     * @var mixed|null
     */
    protected $oidcIssuerUrl;
    /**
     * @var mixed|null
     */
    protected $oidcClientId;
    /**
     * @var bool|null
     */
    protected $oidcMultiTenant;
    /**
     * @var mixed|null
     */
    protected $oidcIssuerPattern;
    /**
     * @var bool|null
     */
    protected $hasClientSecret;
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
    public function getConfigType(): ?string
    {
        return $this->configType;
    }
    /**
     * @param string|null $configType
     *
     * @return self
     */
    public function setConfigType(?string $configType): self
    {
        $this->initialized['configType'] = true;
        $this->configType = $configType;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getPrimaryClientId()
    {
        return $this->primaryClientId;
    }
    /**
     * @param mixed $primaryClientId
     *
     * @return self
     */
    public function setPrimaryClientId($primaryClientId): self
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
    public function getAuthProvider(): ?string
    {
        return $this->authProvider;
    }
    /**
     * @param string|null $authProvider
     *
     * @return self
     */
    public function setAuthProvider(?string $authProvider): self
    {
        $this->initialized['authProvider'] = true;
        $this->authProvider = $authProvider;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getOidcIssuerUrl()
    {
        return $this->oidcIssuerUrl;
    }
    /**
     * @param mixed $oidcIssuerUrl
     *
     * @return self
     */
    public function setOidcIssuerUrl($oidcIssuerUrl): self
    {
        $this->initialized['oidcIssuerUrl'] = true;
        $this->oidcIssuerUrl = $oidcIssuerUrl;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getOidcClientId()
    {
        return $this->oidcClientId;
    }
    /**
     * @param mixed $oidcClientId
     *
     * @return self
     */
    public function setOidcClientId($oidcClientId): self
    {
        $this->initialized['oidcClientId'] = true;
        $this->oidcClientId = $oidcClientId;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getOidcMultiTenant(): ?bool
    {
        return $this->oidcMultiTenant;
    }
    /**
     * @param bool|null $oidcMultiTenant
     *
     * @return self
     */
    public function setOidcMultiTenant(?bool $oidcMultiTenant): self
    {
        $this->initialized['oidcMultiTenant'] = true;
        $this->oidcMultiTenant = $oidcMultiTenant;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getOidcIssuerPattern()
    {
        return $this->oidcIssuerPattern;
    }
    /**
     * @param mixed $oidcIssuerPattern
     *
     * @return self
     */
    public function setOidcIssuerPattern($oidcIssuerPattern): self
    {
        $this->initialized['oidcIssuerPattern'] = true;
        $this->oidcIssuerPattern = $oidcIssuerPattern;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getHasClientSecret(): ?bool
    {
        return $this->hasClientSecret;
    }
    /**
     * @param bool|null $hasClientSecret
     *
     * @return self
     */
    public function setHasClientSecret(?bool $hasClientSecret): self
    {
        $this->initialized['hasClientSecret'] = true;
        $this->hasClientSecret = $hasClientSecret;
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