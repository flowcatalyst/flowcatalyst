<?php

namespace FlowCatalyst\Generated\Model;

class WindowedMetrics extends \ArrayObject
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
     * Messages failed in this window
     *
     * @var int|null
     */
    protected $failureCount;
    /**
     * Processing time metrics with percentiles
     *
     * @var ProcessingTimeMetrics|null
     */
    protected $processingTime;
    /**
     * Messages rate limited in this window
     *
     * @var int|null
     */
    protected $rateLimitedCount;
    /**
     * Messages processed successfully in this window
     *
     * @var int|null
     */
    protected $successCount;
    /**
     * Success rate in this window (0.0 - 1.0)
     *
     * @var float|null
     */
    protected $successRate;
    /**
     * Throughput (messages per second)
     *
     * @var float|null
     */
    protected $throughputPerSec;
    /**
     * Window duration in seconds
     *
     * @var int|null
     */
    protected $windowDurationSecs;
    /**
     * Window start time
     *
     * @var \DateTime|null
     */
    protected $windowStart;
    /**
     * Messages failed in this window
     *
     * @return int|null
     */
    public function getFailureCount(): ?int
    {
        return $this->failureCount;
    }
    /**
     * Messages failed in this window
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
     * Messages rate limited in this window
     *
     * @return int|null
     */
    public function getRateLimitedCount(): ?int
    {
        return $this->rateLimitedCount;
    }
    /**
     * Messages rate limited in this window
     *
     * @param int|null $rateLimitedCount
     *
     * @return self
     */
    public function setRateLimitedCount(?int $rateLimitedCount): self
    {
        $this->initialized['rateLimitedCount'] = true;
        $this->rateLimitedCount = $rateLimitedCount;
        return $this;
    }
    /**
     * Messages processed successfully in this window
     *
     * @return int|null
     */
    public function getSuccessCount(): ?int
    {
        return $this->successCount;
    }
    /**
     * Messages processed successfully in this window
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
     * Success rate in this window (0.0 - 1.0)
     *
     * @return float|null
     */
    public function getSuccessRate(): ?float
    {
        return $this->successRate;
    }
    /**
     * Success rate in this window (0.0 - 1.0)
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
     * Throughput (messages per second)
     *
     * @return float|null
     */
    public function getThroughputPerSec(): ?float
    {
        return $this->throughputPerSec;
    }
    /**
     * Throughput (messages per second)
     *
     * @param float|null $throughputPerSec
     *
     * @return self
     */
    public function setThroughputPerSec(?float $throughputPerSec): self
    {
        $this->initialized['throughputPerSec'] = true;
        $this->throughputPerSec = $throughputPerSec;
        return $this;
    }
    /**
     * Window duration in seconds
     *
     * @return int|null
     */
    public function getWindowDurationSecs(): ?int
    {
        return $this->windowDurationSecs;
    }
    /**
     * Window duration in seconds
     *
     * @param int|null $windowDurationSecs
     *
     * @return self
     */
    public function setWindowDurationSecs(?int $windowDurationSecs): self
    {
        $this->initialized['windowDurationSecs'] = true;
        $this->windowDurationSecs = $windowDurationSecs;
        return $this;
    }
    /**
     * Window start time
     *
     * @return \DateTime|null
     */
    public function getWindowStart(): ?\DateTime
    {
        return $this->windowStart;
    }
    /**
     * Window start time
     *
     * @param \DateTime|null $windowStart
     *
     * @return self
     */
    public function setWindowStart(?\DateTime $windowStart): self
    {
        $this->initialized['windowStart'] = true;
        $this->windowStart = $windowStart;
        return $this;
    }
}