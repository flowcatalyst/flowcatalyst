<?php

namespace FlowCatalyst\Generated\Model;

class PoolStats extends \ArrayObject
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
     * @var int|null
     */
    protected $activeWorkers;
    /**
     * @var int|null
     */
    protected $concurrency;
    /**
     * @var bool|null
     */
    protected $isRateLimited;
    /**
     * @var int|null
     */
    protected $messageGroupCount;
    /**
     * @var mixed|null
     */
    protected $metrics;
    /**
     * @var string|null
     */
    protected $poolCode;
    /**
     * @var int|null
     */
    protected $queueCapacity;
    /**
     * @var int|null
     */
    protected $queueSize;
    /**
     * @var int|null
     */
    protected $rateLimitPerMinute;
    /**
     * @return int|null
     */
    public function getActiveWorkers(): ?int
    {
        return $this->activeWorkers;
    }
    /**
     * @param int|null $activeWorkers
     *
     * @return self
     */
    public function setActiveWorkers(?int $activeWorkers): self
    {
        $this->initialized['activeWorkers'] = true;
        $this->activeWorkers = $activeWorkers;
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
     * @return bool|null
     */
    public function getIsRateLimited(): ?bool
    {
        return $this->isRateLimited;
    }
    /**
     * @param bool|null $isRateLimited
     *
     * @return self
     */
    public function setIsRateLimited(?bool $isRateLimited): self
    {
        $this->initialized['isRateLimited'] = true;
        $this->isRateLimited = $isRateLimited;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getMessageGroupCount(): ?int
    {
        return $this->messageGroupCount;
    }
    /**
     * @param int|null $messageGroupCount
     *
     * @return self
     */
    public function setMessageGroupCount(?int $messageGroupCount): self
    {
        $this->initialized['messageGroupCount'] = true;
        $this->messageGroupCount = $messageGroupCount;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getMetrics()
    {
        return $this->metrics;
    }
    /**
     * @param mixed $metrics
     *
     * @return self
     */
    public function setMetrics($metrics): self
    {
        $this->initialized['metrics'] = true;
        $this->metrics = $metrics;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getPoolCode(): ?string
    {
        return $this->poolCode;
    }
    /**
     * @param string|null $poolCode
     *
     * @return self
     */
    public function setPoolCode(?string $poolCode): self
    {
        $this->initialized['poolCode'] = true;
        $this->poolCode = $poolCode;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getQueueCapacity(): ?int
    {
        return $this->queueCapacity;
    }
    /**
     * @param int|null $queueCapacity
     *
     * @return self
     */
    public function setQueueCapacity(?int $queueCapacity): self
    {
        $this->initialized['queueCapacity'] = true;
        $this->queueCapacity = $queueCapacity;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getQueueSize(): ?int
    {
        return $this->queueSize;
    }
    /**
     * @param int|null $queueSize
     *
     * @return self
     */
    public function setQueueSize(?int $queueSize): self
    {
        $this->initialized['queueSize'] = true;
        $this->queueSize = $queueSize;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getRateLimitPerMinute(): ?int
    {
        return $this->rateLimitPerMinute;
    }
    /**
     * @param int|null $rateLimitPerMinute
     *
     * @return self
     */
    public function setRateLimitPerMinute(?int $rateLimitPerMinute): self
    {
        $this->initialized['rateLimitPerMinute'] = true;
        $this->rateLimitPerMinute = $rateLimitPerMinute;
        return $this;
    }
}