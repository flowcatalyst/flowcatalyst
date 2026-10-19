<?php

namespace FlowCatalyst\Generated\Model;

class UpdateFunctionRequest extends \ArrayObject
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
     * Set true to move the function back to the default runner pool
     *
     * @var bool|null
     */
    protected $clearPool;
    /**
     * Set true to return the function to its application's service account
     *
     * @var bool|null
     */
    protected $clearServiceAccount;
    /**
     * @var string|null
     */
    protected $description;
    /**
     * @var LimitsDTO|null
     */
    protected $limits;
    /**
     * @var string|null
     */
    protected $pool;
    /**
     * Set the webhook verifier service account (id or code); must exist, be active and be reachable by the caller
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
     * Set true to move the function back to the default runner pool
     *
     * @return bool|null
     */
    public function getClearPool(): ?bool
    {
        return $this->clearPool;
    }
    /**
     * Set true to move the function back to the default runner pool
     *
     * @param bool|null $clearPool
     *
     * @return self
     */
    public function setClearPool(?bool $clearPool): self
    {
        $this->initialized['clearPool'] = true;
        $this->clearPool = $clearPool;
        return $this;
    }
    /**
     * Set true to return the function to its application's service account
     *
     * @return bool|null
     */
    public function getClearServiceAccount(): ?bool
    {
        return $this->clearServiceAccount;
    }
    /**
     * Set true to return the function to its application's service account
     *
     * @param bool|null $clearServiceAccount
     *
     * @return self
     */
    public function setClearServiceAccount(?bool $clearServiceAccount): self
    {
        $this->initialized['clearServiceAccount'] = true;
        $this->clearServiceAccount = $clearServiceAccount;
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
     * @return string|null
     */
    public function getPool(): ?string
    {
        return $this->pool;
    }
    /**
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
     * Set the webhook verifier service account (id or code); must exist, be active and be reachable by the caller
     *
     * @return string|null
     */
    public function getServiceAccountId(): ?string
    {
        return $this->serviceAccountId;
    }
    /**
     * Set the webhook verifier service account (id or code); must exist, be active and be reachable by the caller
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