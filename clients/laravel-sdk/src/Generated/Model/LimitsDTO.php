<?php

namespace FlowCatalyst\Generated\Model;

class LimitsDTO
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
    protected $maxBodyBytes;
    /**
     * @var int|null
     */
    protected $maxConcurrency;
    /**
     * @var int|null
     */
    protected $memoryMb;
    /**
     * @var int|null
     */
    protected $timeoutMs;
    /**
     * @return int|null
     */
    public function getMaxBodyBytes(): ?int
    {
        return $this->maxBodyBytes;
    }
    /**
     * @param int|null $maxBodyBytes
     *
     * @return self
     */
    public function setMaxBodyBytes(?int $maxBodyBytes): self
    {
        $this->initialized['maxBodyBytes'] = true;
        $this->maxBodyBytes = $maxBodyBytes;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getMaxConcurrency(): ?int
    {
        return $this->maxConcurrency;
    }
    /**
     * @param int|null $maxConcurrency
     *
     * @return self
     */
    public function setMaxConcurrency(?int $maxConcurrency): self
    {
        $this->initialized['maxConcurrency'] = true;
        $this->maxConcurrency = $maxConcurrency;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getMemoryMb(): ?int
    {
        return $this->memoryMb;
    }
    /**
     * @param int|null $memoryMb
     *
     * @return self
     */
    public function setMemoryMb(?int $memoryMb): self
    {
        $this->initialized['memoryMb'] = true;
        $this->memoryMb = $memoryMb;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getTimeoutMs(): ?int
    {
        return $this->timeoutMs;
    }
    /**
     * @param int|null $timeoutMs
     *
     * @return self
     */
    public function setTimeoutMs(?int $timeoutMs): self
    {
        $this->initialized['timeoutMs'] = true;
        $this->timeoutMs = $timeoutMs;
        return $this;
    }
}