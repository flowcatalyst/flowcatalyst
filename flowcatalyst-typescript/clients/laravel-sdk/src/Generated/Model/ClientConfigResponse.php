<?php

namespace FlowCatalyst\Generated\Model;

class ClientConfigResponse extends \ArrayObject
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
     * Config ID
     *
     * @var string|null
     */
    protected $id;
    /**
     * Application ID
     *
     * @var string|null
     */
    protected $applicationId;
    /**
     * Client ID
     *
     * @var string|null
     */
    protected $clientId;
    /**
     * Client name
     *
     * @var string|null
     */
    protected $clientName;
    /**
     * Client identifier
     *
     * @var string|null
     */
    protected $clientIdentifier;
    /**
     * Whether enabled for this client
     *
     * @var bool|null
     */
    protected $enabled;
    /**
     * Base URL override for this client
     *
     * @var string|null
     */
    protected $baseUrlOverride;
    /**
     * Website override for this client
     *
     * @var string|null
     */
    protected $websiteOverride;
    /**
     * Effective base URL (override or default)
     *
     * @var string|null
     */
    protected $effectiveBaseUrl;
    /**
     * Effective website (override or default)
     *
     * @var string|null
     */
    protected $effectiveWebsite;
    /**
     * Additional configuration
     *
     * @var array<string, mixed>|null
     */
    protected $config;
    /**
     * Config ID
     *
     * @return string|null
     */
    public function getId(): ?string
    {
        return $this->id;
    }
    /**
     * Config ID
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
     * Application ID
     *
     * @return string|null
     */
    public function getApplicationId(): ?string
    {
        return $this->applicationId;
    }
    /**
     * Application ID
     *
     * @param string|null $applicationId
     *
     * @return self
     */
    public function setApplicationId(?string $applicationId): self
    {
        $this->initialized['applicationId'] = true;
        $this->applicationId = $applicationId;
        return $this;
    }
    /**
     * Client ID
     *
     * @return string|null
     */
    public function getClientId(): ?string
    {
        return $this->clientId;
    }
    /**
     * Client ID
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
     * Client name
     *
     * @return string|null
     */
    public function getClientName(): ?string
    {
        return $this->clientName;
    }
    /**
     * Client name
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
     * Client identifier
     *
     * @return string|null
     */
    public function getClientIdentifier(): ?string
    {
        return $this->clientIdentifier;
    }
    /**
     * Client identifier
     *
     * @param string|null $clientIdentifier
     *
     * @return self
     */
    public function setClientIdentifier(?string $clientIdentifier): self
    {
        $this->initialized['clientIdentifier'] = true;
        $this->clientIdentifier = $clientIdentifier;
        return $this;
    }
    /**
     * Whether enabled for this client
     *
     * @return bool|null
     */
    public function getEnabled(): ?bool
    {
        return $this->enabled;
    }
    /**
     * Whether enabled for this client
     *
     * @param bool|null $enabled
     *
     * @return self
     */
    public function setEnabled(?bool $enabled): self
    {
        $this->initialized['enabled'] = true;
        $this->enabled = $enabled;
        return $this;
    }
    /**
     * Base URL override for this client
     *
     * @return string|null
     */
    public function getBaseUrlOverride(): ?string
    {
        return $this->baseUrlOverride;
    }
    /**
     * Base URL override for this client
     *
     * @param string|null $baseUrlOverride
     *
     * @return self
     */
    public function setBaseUrlOverride(?string $baseUrlOverride): self
    {
        $this->initialized['baseUrlOverride'] = true;
        $this->baseUrlOverride = $baseUrlOverride;
        return $this;
    }
    /**
     * Website override for this client
     *
     * @return string|null
     */
    public function getWebsiteOverride(): ?string
    {
        return $this->websiteOverride;
    }
    /**
     * Website override for this client
     *
     * @param string|null $websiteOverride
     *
     * @return self
     */
    public function setWebsiteOverride(?string $websiteOverride): self
    {
        $this->initialized['websiteOverride'] = true;
        $this->websiteOverride = $websiteOverride;
        return $this;
    }
    /**
     * Effective base URL (override or default)
     *
     * @return string|null
     */
    public function getEffectiveBaseUrl(): ?string
    {
        return $this->effectiveBaseUrl;
    }
    /**
     * Effective base URL (override or default)
     *
     * @param string|null $effectiveBaseUrl
     *
     * @return self
     */
    public function setEffectiveBaseUrl(?string $effectiveBaseUrl): self
    {
        $this->initialized['effectiveBaseUrl'] = true;
        $this->effectiveBaseUrl = $effectiveBaseUrl;
        return $this;
    }
    /**
     * Effective website (override or default)
     *
     * @return string|null
     */
    public function getEffectiveWebsite(): ?string
    {
        return $this->effectiveWebsite;
    }
    /**
     * Effective website (override or default)
     *
     * @param string|null $effectiveWebsite
     *
     * @return self
     */
    public function setEffectiveWebsite(?string $effectiveWebsite): self
    {
        $this->initialized['effectiveWebsite'] = true;
        $this->effectiveWebsite = $effectiveWebsite;
        return $this;
    }
    /**
     * Additional configuration
     *
     * @return array<string, mixed>|null
     */
    public function getConfig(): ?iterable
    {
        return $this->config;
    }
    /**
     * Additional configuration
     *
     * @param array<string, mixed>|null $config
     *
     * @return self
     */
    public function setConfig(?iterable $config): self
    {
        $this->initialized['config'] = true;
        $this->config = $config;
        return $this;
    }
}