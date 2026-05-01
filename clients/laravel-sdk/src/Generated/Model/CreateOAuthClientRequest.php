<?php

namespace FlowCatalyst\Generated\Model;

class CreateOAuthClientRequest extends \ArrayObject
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
     * Application IDs this client can access
     *
     * @var list<string>|null
     */
    protected $applicationIds;
    /**
     * OAuth client_id (public identifier). Auto-generated if not provided.
     *
     * @var string|null
     */
    protected $clientId;
    /**
     * Human-readable name
     *
     * @var string|null
     */
    protected $clientName;
    /**
     * Client type (PUBLIC or CONFIDENTIAL)
     *
     * @var string|null
     */
    protected $clientType;
    /**
     * Allowed grant types
     *
     * @var list<string>|null
     */
    protected $grantTypes;
    /**
     * Whether PKCE is required
     *
     * @var bool|null
     */
    protected $pkceRequired;
    /**
     * Allowed redirect URIs
     *
     * @var list<string>|null
     */
    protected $redirectUris;
    /**
     * Application IDs this client can access
     *
     * @return list<string>|null
     */
    public function getApplicationIds(): ?array
    {
        return $this->applicationIds;
    }
    /**
     * Application IDs this client can access
     *
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
     * OAuth client_id (public identifier). Auto-generated if not provided.
     *
     * @return string|null
     */
    public function getClientId(): ?string
    {
        return $this->clientId;
    }
    /**
     * OAuth client_id (public identifier). Auto-generated if not provided.
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
     * Human-readable name
     *
     * @return string|null
     */
    public function getClientName(): ?string
    {
        return $this->clientName;
    }
    /**
     * Human-readable name
     *
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
     * Client type (PUBLIC or CONFIDENTIAL)
     *
     * @return string|null
     */
    public function getClientType(): ?string
    {
        return $this->clientType;
    }
    /**
     * Client type (PUBLIC or CONFIDENTIAL)
     *
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
     * Allowed grant types
     *
     * @return list<string>|null
     */
    public function getGrantTypes(): ?array
    {
        return $this->grantTypes;
    }
    /**
     * Allowed grant types
     *
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
     * Whether PKCE is required
     *
     * @return bool|null
     */
    public function getPkceRequired(): ?bool
    {
        return $this->pkceRequired;
    }
    /**
     * Whether PKCE is required
     *
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
     * Allowed redirect URIs
     *
     * @return list<string>|null
     */
    public function getRedirectUris(): ?array
    {
        return $this->redirectUris;
    }
    /**
     * Allowed redirect URIs
     *
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
}