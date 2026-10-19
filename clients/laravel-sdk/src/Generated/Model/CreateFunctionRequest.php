<?php

namespace FlowCatalyst\Generated\Model;

class CreateFunctionRequest extends \ArrayObject
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
     * Owning application id
     *
     * @var string|null
     */
    protected $applicationId;
    /**
     * @var string|null
     */
    protected $clientId;
    /**
     * @var string|null
     */
    protected $description;
    /**
     * @var LimitsDTO|null
     */
    protected $limits;
    /**
     * Function name (lowercase, alphanumeric, hyphens; matches ^[a-z][a-z0-9-]{0,62}$)
     *
     * @var string|null
     */
    protected $name;
    /**
     * The runner pool that hosts the function (FC_FUNCTIONS_POOL on its runners); omitted means the default pool. Not the dispatch pool, which is always the function's own.
     *
     * @var string|null
     */
    protected $pool;
    /**
     * Optional webhook verifier identity: a service account id OR code. Its signing secret is what this function's webhook endpoints verify deliveries with. Omitted means the application's service account.
     *
     * @var string|null
     */
    protected $serviceAccountId;
    /**
     * @var bool|null
     */
    protected $warm;
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
     * Owning application id
     *
     * @return string|null
     */
    public function getApplicationId(): ?string
    {
        return $this->applicationId;
    }
    /**
     * Owning application id
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
    public function getDescription(): ?string
    {
        return $this->description;
    }
    /**
     * @param string|null $description
     *
     * @return self
     */
    public function setDescription(?string $description): self
    {
        $this->initialized['description'] = true;
        $this->description = $description;
        return $this;
    }
    /**
     * @return LimitsDTO|null
     */
    public function getLimits(): ?LimitsDTO
    {
        return $this->limits;
    }
    /**
     * @param LimitsDTO|null $limits
     *
     * @return self
     */
    public function setLimits(?LimitsDTO $limits): self
    {
        $this->initialized['limits'] = true;
        $this->limits = $limits;
        return $this;
    }
    /**
     * Function name (lowercase, alphanumeric, hyphens; matches ^[a-z][a-z0-9-]{0,62}$)
     *
     * @return string|null
     */
    public function getName(): ?string
    {
        return $this->name;
    }
    /**
     * Function name (lowercase, alphanumeric, hyphens; matches ^[a-z][a-z0-9-]{0,62}$)
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
     * The runner pool that hosts the function (FC_FUNCTIONS_POOL on its runners); omitted means the default pool. Not the dispatch pool, which is always the function's own.
     *
     * @return string|null
     */
    public function getPool(): ?string
    {
        return $this->pool;
    }
    /**
     * The runner pool that hosts the function (FC_FUNCTIONS_POOL on its runners); omitted means the default pool. Not the dispatch pool, which is always the function's own.
     *
     * @param string|null $pool
     *
     * @return self
     */
    public function setPool(?string $pool): self
    {
        $this->initialized['pool'] = true;
        $this->pool = $pool;
        return $this;
    }
    /**
     * Optional webhook verifier identity: a service account id OR code. Its signing secret is what this function's webhook endpoints verify deliveries with. Omitted means the application's service account.
     *
     * @return string|null
     */
    public function getServiceAccountId(): ?string
    {
        return $this->serviceAccountId;
    }
    /**
     * Optional webhook verifier identity: a service account id OR code. Its signing secret is what this function's webhook endpoints verify deliveries with. Omitted means the application's service account.
     *
     * @param string|null $serviceAccountId
     *
     * @return self
     */
    public function setServiceAccountId(?string $serviceAccountId): self
    {
        $this->initialized['serviceAccountId'] = true;
        $this->serviceAccountId = $serviceAccountId;
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
}