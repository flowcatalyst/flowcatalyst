<?php

namespace FlowCatalyst\Generated\Model;

class UpdateOAuthClientRequest extends \ArrayObject
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
     * @var list<string>|null
     */
    protected $allowedOrigins;
    /**
     * @var list<string>|null
     */
    protected $applicationIds;
    /**
     * @var string|null
     */
    protected $clientName;
    /**
     * @var list<string>|null
     */
    protected $defaultScopes;
    /**
     * @var list<string>|null
     */
    protected $grantTypes;
    /**
     * @var bool|null
     */
    protected $pkceRequired;
    /**
     * @var list<string>|null
     */
    protected $postLogoutRedirectUris;
    /**
     * @var list<string>|null
     */
    protected $redirectUris;
    /**
     * @var list<string>|null
     */
    protected $scopes;
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
    public function getPostLogoutRedirectUris(): ?array
    {
        return $this->postLogoutRedirectUris;
    }
    /**
     * @param list<string>|null $postLogoutRedirectUris
     *
     * @return self
     */
    public function setPostLogoutRedirectUris(?array $postLogoutRedirectUris): self
    {
        $this->initialized['postLogoutRedirectUris'] = true;
        $this->postLogoutRedirectUris = $postLogoutRedirectUris;
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
    public function getScopes(): ?array
    {
        return $this->scopes;
    }
    /**
     * @param list<string>|null $scopes
     *
     * @return self
     */
    public function setScopes(?array $scopes): self
    {
        $this->initialized['scopes'] = true;
        $this->scopes = $scopes;
        return $this;
    }
}