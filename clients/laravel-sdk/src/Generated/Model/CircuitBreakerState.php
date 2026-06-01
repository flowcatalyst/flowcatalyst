<?php

namespace FlowCatalyst\Generated\Model;

class CircuitBreakerState extends \ArrayObject
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
     * Failure count
     *
     * @var int|null
     */
    protected $failureCount;
    /**
     * Last failure time
     *
     * @var string|null
     */
    protected $lastFailure;
    /**
     * Time until reset (if open)
     *
     * @var string|null
     */
    protected $resetAt;
    /**
     * Current state (CLOSED, OPEN, HALF_OPEN)
     *
     * @var string|null
     */
    protected $state;
    /**
     * Success count since last failure
     *
     * @var int|null
     */
    protected $successCount;
    /**
     * Target identifier
     *
     * @var string|null
     */
    protected $target;
    /**
     * Failure count
     *
     * @return int|null
     */
    public function getFailureCount(): ?int
    {
        return $this->failureCount;
    }
    /**
     * Failure count
     *
     * @param int|null $failureCount
     *
     * @return self
     */
    public function setFailureCount(?int $failureCount): self
    {
        $this->initialized['failureCount'] = true;
        $this->failureCount = $failureCount;
        return $this;
    }
    /**
     * Last failure time
     *
     * @return string|null
     */
    public function getLastFailure(): ?string
    {
        return $this->lastFailure;
    }
    /**
     * Last failure time
     *
     * @param string|null $lastFailure
     *
     * @return self
     */
    public function setLastFailure(?string $lastFailure): self
    {
        $this->initialized['lastFailure'] = true;
        $this->lastFailure = $lastFailure;
        return $this;
    }
    /**
     * Time until reset (if open)
     *
     * @return string|null
     */
    public function getResetAt(): ?string
    {
        return $this->resetAt;
    }
    /**
     * Time until reset (if open)
     *
     * @param string|null $resetAt
     *
     * @return self
     */
    public function setResetAt(?string $resetAt): self
    {
        $this->initialized['resetAt'] = true;
        $this->resetAt = $resetAt;
        return $this;
    }
    /**
     * Current state (CLOSED, OPEN, HALF_OPEN)
     *
     * @return string|null
     */
    public function getState(): ?string
    {
        return $this->state;
    }
    /**
     * Current state (CLOSED, OPEN, HALF_OPEN)
     *
     * @param string|null $state
     *
     * @return self
     */
    public function setState(?string $state): self
    {
        $this->initialized['state'] = true;
        $this->state = $state;
        return $this;
    }
    /**
     * Success count since last failure
     *
     * @return int|null
     */
    public function getSuccessCount(): ?int
    {
        return $this->successCount;
    }
    /**
     * Success count since last failure
     *
     * @param int|null $successCount
     *
     * @return self
     */
    public function setSuccessCount(?int $successCount): self
    {
        $this->initialized['successCount'] = true;
        $this->successCount = $successCount;
        return $this;
    }
    /**
     * Target identifier
     *
     * @return string|null
     */
    public function getTarget(): ?string
    {
        return $this->target;
    }
    /**
     * Target identifier
     *
     * @param string|null $target
     *
     * @return self
     */
    public function setTarget(?string $target): self
    {
        $this->initialized['target'] = true;
        $this->target = $target;
        return $this;
    }
}