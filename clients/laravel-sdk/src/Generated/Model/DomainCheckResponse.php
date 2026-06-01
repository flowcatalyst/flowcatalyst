<?php

namespace FlowCatalyst\Generated\Model;

class DomainCheckResponse extends \ArrayObject
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
     * Authentication method
     *
     * @var string|null
     */
    protected $authMethod;
    /**
     * Authorization URL if external IDP
     *
     * @var string|null
     */
    protected $authorizationUrl;
    /**
     * The email domain
     *
     * @var string|null
     */
    protected $domain;
    /**
     * Provider ID if external IDP is required
     *
     * @var string|null
     */
    protected $providerId;
    /**
     * Authentication method
     *
     * @return string|null
     */
    public function getAuthMethod(): ?string
    {
        return $this->authMethod;
    }
    /**
     * Authentication method
     *
     * @param string|null $authMethod
     *
     * @return self
     */
    public function setAuthMethod(?string $authMethod): self
    {
        $this->initialized['authMethod'] = true;
        $this->authMethod = $authMethod;
        return $this;
    }
    /**
     * Authorization URL if external IDP
     *
     * @return string|null
     */
    public function getAuthorizationUrl(): ?string
    {
        return $this->authorizationUrl;
    }
    /**
     * Authorization URL if external IDP
     *
     * @param string|null $authorizationUrl
     *
     * @return self
     */
    public function setAuthorizationUrl(?string $authorizationUrl): self
    {
        $this->initialized['authorizationUrl'] = true;
        $this->authorizationUrl = $authorizationUrl;
        return $this;
    }
    /**
     * The email domain
     *
     * @return string|null
     */
    public function getDomain(): ?string
    {
        return $this->domain;
    }
    /**
     * The email domain
     *
     * @param string|null $domain
     *
     * @return self
     */
    public function setDomain(?string $domain): self
    {
        $this->initialized['domain'] = true;
        $this->domain = $domain;
        return $this;
    }
    /**
     * Provider ID if external IDP is required
     *
     * @return string|null
     */
    public function getProviderId(): ?string
    {
        return $this->providerId;
    }
    /**
     * Provider ID if external IDP is required
     *
     * @param string|null $providerId
     *
     * @return self
     */
    public function setProviderId(?string $providerId): self
    {
        $this->initialized['providerId'] = true;
        $this->providerId = $providerId;
        return $this;
    }
}