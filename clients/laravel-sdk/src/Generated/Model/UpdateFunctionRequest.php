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