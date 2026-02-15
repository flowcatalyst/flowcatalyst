<?php

namespace FlowCatalyst\Generated\Model;

class ServiceAccountInfo extends \ArrayObject
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
     * Principal ID of the service account
     *
     * @var string|null
     */
    protected $principalId;
    /**
     * Name of the service account
     *
     * @var string|null
     */
    protected $name;
    /**
     * OAuth client information
     *
     * @var OAuthClientInfo|null
     */
    protected $oauthClient;
    /**
     * Principal ID of the service account
     *
     * @return string|null
     */
    public function getPrincipalId(): ?string
    {
        return $this->principalId;
    }
    /**
     * Principal ID of the service account
     *
     * @param string|null $principalId
     *
     * @return self
     */
    public function setPrincipalId(?string $principalId): self
    {
        $this->initialized['principalId'] = true;
        $this->principalId = $principalId;
        return $this;
    }
    /**
     * Name of the service account
     *
     * @return string|null
     */
    public function getName(): ?string
    {
        return $this->name;
    }
    /**
     * Name of the service account
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
     * OAuth client information
     *
     * @return OAuthClientInfo|null
     */
    public function getOauthClient(): ?OAuthClientInfo
    {
        return $this->oauthClient;
    }
    /**
     * OAuth client information
     *
     * @param OAuthClientInfo|null $oauthClient
     *
     * @return self
     */
    public function setOauthClient(?OAuthClientInfo $oauthClient): self
    {
        $this->initialized['oauthClient'] = true;
        $this->oauthClient = $oauthClient;
        return $this;
    }
}