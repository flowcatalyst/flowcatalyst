<?php

namespace FlowCatalyst\Generated\Model;

class UpdateDispatchPoolRequest extends \ArrayObject
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
    protected $name;
    /**
     * @var string|null
     */
    protected $description;
    /**
     * @var int|null
     */
    protected $rateLimit;
    /**
     * @var int|null
     */
    protected $concurrency;
    /**
     * @var string|null
     */
    protected $status;
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
     * @return int|null
     */
    public function getRateLimit(): ?int
    {
        return $this->rateLimit;
    }
    /**
     * @param int|null $rateLimit
     *
     * @return self
     */
    public function setRateLimit(?int $rateLimit): self
    {
        $this->initialized['rateLimit'] = true;
        $this->rateLimit = $rateLimit;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getConcurrency(): ?int
    {
        return $this->concurrency;
    }
    /**
     * @param int|null $concurrency
     *
     * @return self
     */
    public function setConcurrency(?int $concurrency): self
    {
        $this->initialized['concurrency'] = true;
        $this->concurrency = $concurrency;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getStatus(): ?string
    {
        return $this->status;
    }
    /**
     * @param string|null $status
     *
     * @return self
     */
    public function setStatus(?string $status): self
    {
        $this->initialized['status'] = true;
        $this->status = $status;
        return $this;
    }
}