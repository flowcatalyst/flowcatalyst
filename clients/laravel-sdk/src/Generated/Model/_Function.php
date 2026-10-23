<?php

namespace FlowCatalyst\Generated\Model;

class _Function
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
    protected $address;
    /**
     * @var string|null
     */
    protected $applicationId;
    /**
     * @var string|null
     */
    protected $clientId;
    /**
     * @var array<string, string>|null
     */
    protected $config;
    /**
     * @var array<string, string>|null
     */
    protected $db;
    /**
     * @var string|null
     */
    protected $id;
    /**
     * @var Limits|null
     */
    protected $limits;
    /**
     * @var array<string, string>|null
     */
    protected $secrets;
    /**
     * @var list<TraceState>|null
     */
    protected $trace;
    /**
     * @var list<Version>|null
     */
    protected $versions;
    /**
     * @var bool|null
     */
    protected $warm;
    /**
     * @var string|null
     */
    protected $webhookSecret;
    /**
     * @return string|null
     */
    public function getAddress(): ?string
    {
        return $this->address;
    }
    /**
     * @param string|null $address
     *
     * @return self
     */
    public function setAddress(?string $address): self
    {
        $this->initialized['address'] = true;
        $this->address = $address;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getApplicationId(): ?string
    {
        return $this->applicationId;
    }
    /**
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
     * @return array<string, string>|null
     */
    public function getConfig(): ?iterable
    {
        return $this->config;
    }
    /**
     * @param array<string, string>|null $config
     *
     * @return self
     */
    public function setConfig(?iterable $config): self
    {
        $this->initialized['config'] = true;
        $this->config = $config;
        return $this;
    }
    /**
     * @return array<string, string>|null
     */
    public function getDb(): ?iterable
    {
        return $this->db;
    }
    /**
     * @param array<string, string>|null $db
     *
     * @return self
     */
    public function setDb(?iterable $db): self
    {
        $this->initialized['db'] = true;
        $this->db = $db;
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
     * @return Limits|null
     */
    public function getLimits(): ?Limits
    {
        return $this->limits;
    }
    /**
     * @param Limits|null $limits
     *
     * @return self
     */
    public function setLimits(?Limits $limits): self
    {
        $this->initialized['limits'] = true;
        $this->limits = $limits;
        return $this;
    }
    /**
     * @return array<string, string>|null
     */
    public function getSecrets(): ?iterable
    {
        return $this->secrets;
    }
    /**
     * @param array<string, string>|null $secrets
     *
     * @return self
     */
    public function setSecrets(?iterable $secrets): self
    {
        $this->initialized['secrets'] = true;
        $this->secrets = $secrets;
        return $this;
    }
    /**
     * @return list<TraceState>|null
     */
    public function getTrace(): ?array
    {
        return $this->trace;
    }
    /**
     * @param list<TraceState>|null $trace
     *
     * @return self
     */
    public function setTrace(?array $trace): self
    {
        $this->initialized['trace'] = true;
        $this->trace = $trace;
        return $this;
    }
    /**
     * @return list<Version>|null
     */
    public function getVersions(): ?array
    {
        return $this->versions;
    }
    /**
     * @param list<Version>|null $versions
     *
     * @return self
     */
    public function setVersions(?array $versions): self
    {
        $this->initialized['versions'] = true;
        $this->versions = $versions;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getWarm(): ?bool
    {
        return $this->warm;
    }
    /**
     * @param bool|null $warm
     *
     * @return self
     */
    public function setWarm(?bool $warm): self
    {
        $this->initialized['warm'] = true;
        $this->warm = $warm;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getWebhookSecret(): ?string
    {
        return $this->webhookSecret;
    }
    /**
     * @param string|null $webhookSecret
     *
     * @return self
     */
    public function setWebhookSecret(?string $webhookSecret): self
    {
        $this->initialized['webhookSecret'] = true;
        $this->webhookSecret = $webhookSecret;
        return $this;
    }
}