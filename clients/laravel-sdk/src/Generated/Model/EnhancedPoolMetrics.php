<?php

namespace FlowCatalyst\Generated\Model;

class EnhancedPoolMetrics extends \ArrayObject
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
     * Time-windowed metrics
     *
     * @var WindowedMetrics|null
     */
    protected $last30Min;
    /**
     * Time-windowed metrics
     *
     * @var WindowedMetrics|null
     */
    protected $last5Min;
    /**
     * Processing time metrics with percentiles
     *
     * @var ProcessingTimeMetrics|null
     */
    protected $processingTime;
    /**
     * Success rate (0.0 - 1.0)
     *
     * @var float|null
     */
    protected $successRate;
    /**
     * Total messages failed (all time)
     *
     * @var int|null
     */
    protected $totalFailure;
    /**
     * Total messages rate limited (all time)
     *
     * @var int|null
     */
    protected $totalRateLimited;
    /**
     * Total messages processed successfully (all time)
     *
     * @var int|null
     */
    protected $totalSuccess;
    /**
     * Time-windowed metrics
     *
     * @return WindowedMetrics|null
     */
    public function getLast30Min(): ?WindowedMetrics
    {
        return $this->last30Min;
    }
    /**
     * Time-windowed metrics
     *
     * @param WindowedMetrics|null $last30Min
     *
     * @return self
     */
    public function setLast30Min(?WindowedMetrics $last30Min): self
    {
        $this->initialized['last30Min'] = true;
        $this->last30Min = $last30Min;
        return $this;
    }
    /**
     * Time-windowed metrics
     *
     * @return WindowedMetrics|null
     */
    public function getLast5Min(): ?WindowedMetrics
    {
        return $this->last5Min;
    }
    /**
     * Time-windowed metrics
     *
     * @param WindowedMetrics|null $last5Min
     *
     * @return self
     */
    public function setLast5Min(?WindowedMetrics $last5Min): self
    {
        $this->initialized['last5Min'] = true;
        $this->last5Min = $last5Min;
        return $this;
    }
    /**
     * Processing time metrics with percentiles
     *
     * @return ProcessingTimeMetrics|null
     */
    public function getProcessingTime(): ?ProcessingTimeMetrics
    {
        return $this->processingTime;
    }
    /**
     * Processing time metrics with percentiles
     *
     * @param ProcessingTimeMetrics|null $processingTime
     *
     * @return self
     */
    public function setProcessingTime(?ProcessingTimeMetrics $processingTime): self
    {
        $this->initialized['processingTime'] = true;
        $this->processingTime = $processingTime;
        return $this;
    }
    /**
     * Success rate (0.0 - 1.0)
     *
     * @return float|null
     */
    public function getSuccessRate(): ?float
    {
        return $this->successRate;
    }
    /**
     * Success rate (0.0 - 1.0)
     *
     * @param float|null $successRate
     *
     * @return self
     */
    public function setSuccessRate(?float $successRate): self
    {
        $this->initialized['successRate'] = true;
        $this->successRate = $successRate;
        return $this;
    }
    /**
     * Total messages failed (all time)
     *
     * @return int|null
     */
    public function getTotalFailure(): ?int
    {
        return $this->totalFailure;
    }
    /**
     * Total messages failed (all time)
     *
     * @param int|null $totalFailure
     *
     * @return self
     */
    public function setTotalFailure(?int $totalFailure): self
    {
        $this->initialized['totalFailure'] = true;
        $this->totalFailure = $totalFailure;
        return $this;
    }
    /**
     * Total messages rate limited (all time)
     *
     * @return int|null
     */
    public function getTotalRateLimited(): ?int
    {
        return $this->totalRateLimited;
    }
    /**
     * Total messages rate limited (all time)
     *
     * @param int|null $totalRateLimited
     *
     * @return self
     */
    public function setTotalRateLimited(?int $totalRateLimited): self
    {
        $this->initialized['totalRateLimited'] = true;
        $this->totalRateLimited = $totalRateLimited;
        return $this;
    }
    /**
     * Total messages processed successfully (all time)
     *
     * @return int|null
     */
    public function getTotalSuccess(): ?int
    {
        return $this->totalSuccess;
    }
    /**
     * Total messages processed successfully (all time)
     *
     * @param int|null $totalSuccess
     *
     * @return self
     */
    public function setTotalSuccess(?int $totalSuccess): self
    {
        $this->initialized['totalSuccess'] = true;
        $this->totalSuccess = $totalSuccess;
        return $this;
    }
}