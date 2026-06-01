<?php

namespace FlowCatalyst\Generated\Model;

class PoolStatsResponse extends \ArrayObject
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
     * Aggregate success rate across all pools
     *
     * @var float|null
     */
    protected $aggregateSuccessRate;
    /**
     * Aggregate throughput (messages/sec) across all pools
     *
     * @var float|null
     */
    protected $aggregateThroughputPerSec;
    /**
     * @var list<PoolStats>|null
     */
    protected $pools;
    /**
     * @var int|null
     */
    protected $totalActiveWorkers;
    /**
     * @var int|null
     */
    protected $totalPools;
    /**
     * @var int|null
     */
    protected $totalQueueSize;
    /**
     * Aggregate success rate across all pools
     *
     * @return float|null
     */
    public function getAggregateSuccessRate(): ?float
    {
        return $this->aggregateSuccessRate;
    }
    /**
     * Aggregate success rate across all pools
     *
     * @param float|null $aggregateSuccessRate
     *
     * @return self
     */
    public function setAggregateSuccessRate(?float $aggregateSuccessRate): self
    {
        $this->initialized['aggregateSuccessRate'] = true;
        $this->aggregateSuccessRate = $aggregateSuccessRate;
        return $this;
    }
    /**
     * Aggregate throughput (messages/sec) across all pools
     *
     * @return float|null
     */
    public function getAggregateThroughputPerSec(): ?float
    {
        return $this->aggregateThroughputPerSec;
    }
    /**
     * Aggregate throughput (messages/sec) across all pools
     *
     * @param float|null $aggregateThroughputPerSec
     *
     * @return self
     */
    public function setAggregateThroughputPerSec(?float $aggregateThroughputPerSec): self
    {
        $this->initialized['aggregateThroughputPerSec'] = true;
        $this->aggregateThroughputPerSec = $aggregateThroughputPerSec;
        return $this;
    }
    /**
     * @return list<PoolStats>|null
     */
    public function getPools(): ?array
    {
        return $this->pools;
    }
    /**
     * @param list<PoolStats>|null $pools
     *
     * @return self
     */
    public function setPools(?array $pools): self
    {
        $this->initialized['pools'] = true;
        $this->pools = $pools;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getTotalActiveWorkers(): ?int
    {
        return $this->totalActiveWorkers;
    }
    /**
     * @param int|null $totalActiveWorkers
     *
     * @return self
     */
    public function setTotalActiveWorkers(?int $totalActiveWorkers): self
    {
        $this->initialized['totalActiveWorkers'] = true;
        $this->totalActiveWorkers = $totalActiveWorkers;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getTotalPools(): ?int
    {
        return $this->totalPools;
    }
    /**
     * @param int|null $totalPools
     *
     * @return self
     */
    public function setTotalPools(?int $totalPools): self
    {
        $this->initialized['totalPools'] = true;
        $this->totalPools = $totalPools;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getTotalQueueSize(): ?int
    {
        return $this->totalQueueSize;
    }
    /**
     * @param int|null $totalQueueSize
     *
     * @return self
     */
    public function setTotalQueueSize(?int $totalQueueSize): self
    {
        $this->initialized['totalQueueSize'] = true;
        $this->totalQueueSize = $totalQueueSize;
        return $this;
    }
}