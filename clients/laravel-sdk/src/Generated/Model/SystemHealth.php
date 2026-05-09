<?php

namespace FlowCatalyst\Generated\Model;

class SystemHealth extends \ArrayObject
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
     * @var float|null
     */
    protected $cpuUsagePercent;
    /**
     * @var int|null
     */
    protected $memoryUsedMb;
    /**
     * @var string|null
     */
    protected $status;
    /**
     * @var int|null
     */
    protected $uptimeSeconds;
    /**
     * @return float|null
     */
    public function getCpuUsagePercent(): ?float
    {
        return $this->cpuUsagePercent;
    }
    /**
     * @param float|null $cpuUsagePercent
     *
     * @return self
     */
    public function setCpuUsagePercent(?float $cpuUsagePercent): self
    {
        $this->initialized['cpuUsagePercent'] = true;
        $this->cpuUsagePercent = $cpuUsagePercent;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getMemoryUsedMb(): ?int
    {
        return $this->memoryUsedMb;
    }
    /**
     * @param int|null $memoryUsedMb
     *
     * @return self
     */
    public function setMemoryUsedMb(?int $memoryUsedMb): self
    {
        $this->initialized['memoryUsedMb'] = true;
        $this->memoryUsedMb = $memoryUsedMb;
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
    /**
     * @return int|null
     */
    public function getUptimeSeconds(): ?int
    {
        return $this->uptimeSeconds;
    }
    /**
     * @param int|null $uptimeSeconds
     *
     * @return self
     */
    public function setUptimeSeconds(?int $uptimeSeconds): self
    {
        $this->initialized['uptimeSeconds'] = true;
        $this->uptimeSeconds = $uptimeSeconds;
        return $this;
    }
}