<?php

namespace FlowCatalyst\Generated\Model;

class ProcessingTimeMetrics extends \ArrayObject
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
     * Average processing time in milliseconds
     *
     * @var float|null
     */
    protected $avgMs;
    /**
     * Maximum processing time in milliseconds
     *
     * @var int|null
     */
    protected $maxMs;
    /**
     * Minimum processing time in milliseconds
     *
     * @var int|null
     */
    protected $minMs;
    /**
     * 50th percentile (median) in milliseconds
     *
     * @var int|null
     */
    protected $p50Ms;
    /**
     * 95th percentile in milliseconds
     *
     * @var int|null
     */
    protected $p95Ms;
    /**
     * 99th percentile in milliseconds
     *
     * @var int|null
     */
    protected $p99Ms;
    /**
     * Total samples collected
     *
     * @var int|null
     */
    protected $sampleCount;
    /**
     * Average processing time in milliseconds
     *
     * @return float|null
     */
    public function getAvgMs(): ?float
    {
        return $this->avgMs;
    }
    /**
     * Average processing time in milliseconds
     *
     * @param float|null $avgMs
     *
     * @return self
     */
    public function setAvgMs(?float $avgMs): self
    {
        $this->initialized['avgMs'] = true;
        $this->avgMs = $avgMs;
        return $this;
    }
    /**
     * Maximum processing time in milliseconds
     *
     * @return int|null
     */
    public function getMaxMs(): ?int
    {
        return $this->maxMs;
    }
    /**
     * Maximum processing time in milliseconds
     *
     * @param int|null $maxMs
     *
     * @return self
     */
    public function setMaxMs(?int $maxMs): self
    {
        $this->initialized['maxMs'] = true;
        $this->maxMs = $maxMs;
        return $this;
    }
    /**
     * Minimum processing time in milliseconds
     *
     * @return int|null
     */
    public function getMinMs(): ?int
    {
        return $this->minMs;
    }
    /**
     * Minimum processing time in milliseconds
     *
     * @param int|null $minMs
     *
     * @return self
     */
    public function setMinMs(?int $minMs): self
    {
        $this->initialized['minMs'] = true;
        $this->minMs = $minMs;
        return $this;
    }
    /**
     * 50th percentile (median) in milliseconds
     *
     * @return int|null
     */
    public function getP50Ms(): ?int
    {
        return $this->p50Ms;
    }
    /**
     * 50th percentile (median) in milliseconds
     *
     * @param int|null $p50Ms
     *
     * @return self
     */
    public function setP50Ms(?int $p50Ms): self
    {
        $this->initialized['p50Ms'] = true;
        $this->p50Ms = $p50Ms;
        return $this;
    }
    /**
     * 95th percentile in milliseconds
     *
     * @return int|null
     */
    public function getP95Ms(): ?int
    {
        return $this->p95Ms;
    }
    /**
     * 95th percentile in milliseconds
     *
     * @param int|null $p95Ms
     *
     * @return self
     */
    public function setP95Ms(?int $p95Ms): self
    {
        $this->initialized['p95Ms'] = true;
        $this->p95Ms = $p95Ms;
        return $this;
    }
    /**
     * 99th percentile in milliseconds
     *
     * @return int|null
     */
    public function getP99Ms(): ?int
    {
        return $this->p99Ms;
    }
    /**
     * 99th percentile in milliseconds
     *
     * @param int|null $p99Ms
     *
     * @return self
     */
    public function setP99Ms(?int $p99Ms): self
    {
        $this->initialized['p99Ms'] = true;
        $this->p99Ms = $p99Ms;
        return $this;
    }
    /**
     * Total samples collected
     *
     * @return int|null
     */
    public function getSampleCount(): ?int
    {
        return $this->sampleCount;
    }
    /**
     * Total samples collected
     *
     * @param int|null $sampleCount
     *
     * @return self
     */
    public function setSampleCount(?int $sampleCount): self
    {
        $this->initialized['sampleCount'] = true;
        $this->sampleCount = $sampleCount;
        return $this;
    }
}