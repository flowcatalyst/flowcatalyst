<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminOauthClientsIdGetResponse200 extends \ArrayObject
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
    protected $clientId;
    /**
     * @var string|null
     */
    protected $clientName;
    /**
     * @var string|null
     */
    protected $clientType;
    /**
     * @var bool|null
     */
    protected $hasClientSecret;
    /**
     * @var list<string>|null
     */
    protected $redirectUris;
    /**
     * @var list<string>|null
     */
    protected $allowedOrigins;
    /**
     * @var list<string>|null
     */
    protected $grantTypes;
    /**
     * @var list<string>|null
     */
    protected $defaultScopes;
    /**
     * @var bool|null
     */
    protected $pkceRequired;
    /**
     * @var list<string>|null
     */
    protected $applicationIds;
    /**
     * @var list<ApiAdminOauthClientsIdGetResponse200ApplicationsItem>|null
     */
    protected $applications;
    /**
     * @var mixed|null
     */
    protected $serviceAccountPrincipalId;
    /**
     * @var bool|null
     */
    protected $active;
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
    public function getClientName(): ?string
    {
        return $this->clientName;
    }
    /**
     * @param string|null $clientName
     *
     * @return self
     */
    public function setClientName(?string $clientName): self
    {
        $this->initialized['clientName'] = true;
        $this->clientName = $clientName;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getClientType(): ?string
    {
        return $this->clientType;
    }
    /**
     * @param string|null $clientType
     *
     * @return self
     */
    public function setClientType(?string $clientType): self
    {
        $this->initialized['clientType'] = true;
        $this->clientType = $clientType;
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
     * @return list<string>|null
     */
    public function getRedirectUris(): ?array
    {
        return $this->redirectUris;
    }
    /**
     * @param list<string>|null $redirectUris
     *
     * @return self
     */
    public function setRedirectUris(?array $redirectUris): self
    {
        $this->initialized['redirectUris'] = true;
        $this->redirectUris = $redirectUris;
        return $this;
    }
    /**
     * @return list<string>|null
     */
    public function getAllowedOrigins(): ?array
    {
        return $this->allowedOrigins;
    }
    /**
     * @param list<string>|null $allowedOrigins
     *
     * @return self
     */
    public function setAllowedOrigins(?array $allowedOrigins): self
    {
        $this->initialized['allowedOrigins'] = true;
        $this->allowedOrigins = $allowedOrigins;
        return $this;
    }
    /**
     * @return list<string>|null
     */
    public function getGrantTypes(): ?array
    {
        return $this->grantTypes;
    }
    /**
     * @param list<string>|null $grantTypes
     *
     * @return self
     */
    public function setGrantTypes(?array $grantTypes): self
    {
        $this->initialized['grantTypes'] = true;
        $this->grantTypes = $grantTypes;
        return $this;
    }
    /**
     * @return list<string>|null
     */
    public function getDefaultScopes(): ?array
    {
        return $this->defaultScopes;
    }
    /**
     * @param list<string>|null $defaultScopes
     *
     * @return self
     */
    public function setDefaultScopes(?array $defaultScopes): self
    {
        $this->initialized['defaultScopes'] = true;
        $this->defaultScopes = $defaultScopes;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getPkceRequired(): ?bool
    {
        return $this->pkceRequired;
    }
    /**
     * @param bool|null $pkceRequired
     *
     * @return self
     */
    public function setPkceRequired(?bool $pkceRequired): self
    {
        $this->initialized['pkceRequired'] = true;
        $this->pkceRequired = $pkceRequired;
        return $this;
    }
    /**
     * @return list<string>|null
     */
    public function getApplicationIds(): ?array
    {
        return $this->applicationIds;
    }
    /**
     * @param list<string>|null $applicationIds
     *
     * @return self
     */
    public function setApplicationIds(?array $applicationIds): self
    {
        $this->initialized['applicationIds'] = true;
        $this->applicationIds = $applicationIds;
        return $this;
    }
    /**
     * @return list<ApiAdminOauthClientsIdGetResponse200ApplicationsItem>|null
     */
    public function getApplications(): ?array
    {
        return $this->applications;
    }
    /**
     * @param list<ApiAdminOauthClientsIdGetResponse200ApplicationsItem>|null $applications
     *
     * @return self
     */
    public function setApplications(?array $applications): self
    {
        $this->initialized['applications'] = true;
        $this->applications = $applications;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getServiceAccountPrincipalId()
    {
        return $this->serviceAccountPrincipalId;
    }
    /**
     * @param mixed $serviceAccountPrincipalId
     *
     * @return self
     */
    public function setServiceAccountPrincipalId($serviceAccountPrincipalId): self
    {
        $this->initialized['serviceAccountPrincipalId'] = true;
        $this->serviceAccountPrincipalId = $serviceAccountPrincipalId;
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