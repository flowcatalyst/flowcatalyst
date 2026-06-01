<?php

namespace FlowCatalyst\Generated\Model;

class CircuitBreakersResponse extends \ArrayObject
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
     * @var list<CircuitBreakerState>|null
     */
    protected $breakers;
    /**
     * @var int|null
     */
    protected $totalClosed;
    /**
     * @var int|null
     */
    protected $totalHalfOpen;
    /**
     * @var int|null
     */
    protected $totalOpen;
    /**
     * @return list<CircuitBreakerState>|null
     */
    public function getBreakers(): ?array
    {
        return $this->breakers;
    }
    /**
     * @param list<CircuitBreakerState>|null $breakers
     *
     * @return self
     */
    public function setBreakers(?array $breakers): self
    {
        $this->initialized['breakers'] = true;
        $this->breakers = $breakers;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getTotalClosed(): ?int
    {
        return $this->totalClosed;
    }
    /**
     * @param int|null $totalClosed
     *
     * @return self
     */
    public function setTotalClosed(?int $totalClosed): self
    {
        $this->initialized['totalClosed'] = true;
        $this->totalClosed = $totalClosed;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getTotalHalfOpen(): ?int
    {
        return $this->totalHalfOpen;
    }
    /**
     * @param int|null $totalHalfOpen
     *
     * @return self
     */
    public function setTotalHalfOpen(?int $totalHalfOpen): self
    {
        $this->initialized['totalHalfOpen'] = true;
        $this->totalHalfOpen = $totalHalfOpen;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getTotalOpen(): ?int
    {
        return $this->totalOpen;
    }
    /**
     * @param int|null $totalOpen
     *
     * @return self
     */
    public function setTotalOpen(?int $totalOpen): self
    {
        $this->initialized['totalOpen'] = true;
        $this->totalOpen = $totalOpen;
        return $this;
    }
}