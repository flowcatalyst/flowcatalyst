<?php

namespace FlowCatalyst\Generated\Model;

class RuntimeIngestion
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
    protected $applicationCode;
    /**
     * @var string|null
     */
    protected $clientId;
    /**
     * @var string|null
     */
    protected $clientIdentifier;
    /**
     * @var string|null
     */
    protected $code;
    /**
     * @var string|null
     */
    protected $id;
    /**
     * @var bool|null
     */
    protected $keepFailedPayloads;
    /**
     * @var string|null
     */
    protected $name;
    /**
     * @var mixed|null
     */
    protected $source;
    /**
     * @var mixed|null
     */
    protected $split;
    /**
     * @var Target|null
     */
    protected $target;
    /**
     * The live version; a runtime rebuilds the route when it differs
     *
     * @var int|null
     */
    protected $version;
    /**
     * @return string|null
     */
    public function getApplicationCode(): ?string
    {
        return $this->applicationCode;
    }
    /**
     * @param string|null $applicationCode
     *
     * @return self
     */
    public function setApplicationCode(?string $applicationCode): self
    {
        $this->initialized['applicationCode'] = true;
        $this->applicationCode = $applicationCode;
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
    public function getClientIdentifier(): ?string
    {
        return $this->clientIdentifier;
    }
    /**
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
     * @return string|null
     */
    public function getCode(): ?string
    {
        return $this->code;
    }
    /**
     * @param string|null $code
     *
     * @return self
     */
    public function setCode(?string $code): self
    {
        $this->initialized['code'] = true;
        $this->code = $code;
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
     * @return bool|null
     */
    public function getKeepFailedPayloads(): ?bool
    {
        return $this->keepFailedPayloads;
    }
    /**
     * @param bool|null $keepFailedPayloads
     *
     * @return self
     */
    public function setKeepFailedPayloads(?bool $keepFailedPayloads): self
    {
        $this->initialized['keepFailedPayloads'] = true;
        $this->keepFailedPayloads = $keepFailedPayloads;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getName(): ?string
    {
        return $this->name;
    }
    /**
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
     * @return mixed
     */
    public function getSource()
    {
        return $this->source;
    }
    /**
     * @param mixed $source
     *
     * @return self
     */
    public function setSource($source): self
    {
        $this->initialized['source'] = true;
        $this->source = $source;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getSplit()
    {
        return $this->split;
    }
    /**
     * @param mixed $split
     *
     * @return self
     */
    public function setSplit($split): self
    {
        $this->initialized['split'] = true;
        $this->split = $split;
        return $this;
    }
    /**
     * @return Target|null
     */
    public function getTarget(): ?Target
    {
        return $this->target;
    }
    /**
     * @param Target|null $target
     *
     * @return self
     */
    public function setTarget(?Target $target): self
    {
        $this->initialized['target'] = true;
        $this->target = $target;
        return $this;
    }
    /**
     * The live version; a runtime rebuilds the route when it differs
     *
     * @return int|null
     */
    public function getVersion(): ?int
    {
        return $this->version;
    }
    /**
     * The live version; a runtime rebuilds the route when it differs
     *
     * @param int|null $version
     *
     * @return self
     */
    public function setVersion(?int $version): self
    {
        $this->initialized['version'] = true;
        $this->version = $version;
        return $this;
    }
}