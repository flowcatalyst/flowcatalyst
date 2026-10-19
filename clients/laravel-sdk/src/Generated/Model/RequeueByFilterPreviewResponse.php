<?php

namespace FlowCatalyst\Generated\Model;

class RequeueByFilterPreviewResponse
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
     * A URL to the JSON Schema for this object.
     *
     * @var string|null
     */
    protected $dollarSchema;
    /**
     * count broken down by status
     *
     * @var array<string, int>|null
     */
    protected $byStatus;
    /**
     * COMPLETED jobs the filter matches that the caller can see. A requeue skips them; they are redriven only with redriveCompleted and confirmRedriveCompleted
     *
     * @var int|null
     */
    protected $completedCount;
    /**
     * Jobs the filter matches in the window that the caller can see
     *
     * @var int|null
     */
    protected $count;
    /**
     * How many of them a requeue would reset for this caller (not COMPLETED, not in flight); send it back as confirmCount
     *
     * @var int|null
     */
    protected $eligibleCount;
    /**
     * QUEUED or PROCESSING jobs the filter matches; a requeue never resets them
     *
     * @var int|null
     */
    protected $inFlightCount;
    /**
     * eligibleCount + completedCount: the confirmCount of a request that also redrives COMPLETED jobs
     *
     * @var int|null
     */
    protected $redriveEligibleCount;
    /**
     * @var WindowResponse|null
     */
    protected $window;
    /**
     * A URL to the JSON Schema for this object.
     *
     * @return string|null
     */
    public function getDollarSchema(): ?string
    {
        return $this->dollarSchema;
    }
    /**
     * A URL to the JSON Schema for this object.
     *
     * @param string|null $dollarSchema
     *
     * @return self
     */
    public function setDollarSchema(?string $dollarSchema): self
    {
        $this->initialized['dollarSchema'] = true;
        $this->dollarSchema = $dollarSchema;
        return $this;
    }
    /**
     * count broken down by status
     *
     * @return array<string, int>|null
     */
    public function getByStatus(): ?iterable
    {
        return $this->byStatus;
    }
    /**
     * count broken down by status
     *
     * @param array<string, int>|null $byStatus
     *
     * @return self
     */
    public function setByStatus(?iterable $byStatus): self
    {
        $this->initialized['byStatus'] = true;
        $this->byStatus = $byStatus;
        return $this;
    }
    /**
     * COMPLETED jobs the filter matches that the caller can see. A requeue skips them; they are redriven only with redriveCompleted and confirmRedriveCompleted
     *
     * @return int|null
     */
    public function getCompletedCount(): ?int
    {
        return $this->completedCount;
    }
    /**
     * COMPLETED jobs the filter matches that the caller can see. A requeue skips them; they are redriven only with redriveCompleted and confirmRedriveCompleted
     *
     * @param int|null $completedCount
     *
     * @return self
     */
    public function setCompletedCount(?int $completedCount): self
    {
        $this->initialized['completedCount'] = true;
        $this->completedCount = $completedCount;
        return $this;
    }
    /**
     * Jobs the filter matches in the window that the caller can see
     *
     * @return int|null
     */
    public function getCount(): ?int
    {
        return $this->count;
    }
    /**
     * Jobs the filter matches in the window that the caller can see
     *
     * @param int|null $count
     *
     * @return self
     */
    public function setCount(?int $count): self
    {
        $this->initialized['count'] = true;
        $this->count = $count;
        return $this;
    }
    /**
     * How many of them a requeue would reset for this caller (not COMPLETED, not in flight); send it back as confirmCount
     *
     * @return int|null
     */
    public function getEligibleCount(): ?int
    {
        return $this->eligibleCount;
    }
    /**
     * How many of them a requeue would reset for this caller (not COMPLETED, not in flight); send it back as confirmCount
     *
     * @param int|null $eligibleCount
     *
     * @return self
     */
    public function setEligibleCount(?int $eligibleCount): self
    {
        $this->initialized['eligibleCount'] = true;
        $this->eligibleCount = $eligibleCount;
        return $this;
    }
    /**
     * QUEUED or PROCESSING jobs the filter matches; a requeue never resets them
     *
     * @return int|null
     */
    public function getInFlightCount(): ?int
    {
        return $this->inFlightCount;
    }
    /**
     * QUEUED or PROCESSING jobs the filter matches; a requeue never resets them
     *
     * @param int|null $inFlightCount
     *
     * @return self
     */
    public function setInFlightCount(?int $inFlightCount): self
    {
        $this->initialized['inFlightCount'] = true;
        $this->inFlightCount = $inFlightCount;
        return $this;
    }
    /**
     * eligibleCount + completedCount: the confirmCount of a request that also redrives COMPLETED jobs
     *
     * @return int|null
     */
    public function getRedriveEligibleCount(): ?int
    {
        return $this->redriveEligibleCount;
    }
    /**
     * eligibleCount + completedCount: the confirmCount of a request that also redrives COMPLETED jobs
     *
     * @param int|null $redriveEligibleCount
     *
     * @return self
     */
    public function setRedriveEligibleCount(?int $redriveEligibleCount): self
    {
        $this->initialized['redriveEligibleCount'] = true;
        $this->redriveEligibleCount = $redriveEligibleCount;
        return $this;
    }
    /**
     * @return WindowResponse|null
     */
    public function getWindow(): ?WindowResponse
    {
        return $this->window;
    }
    /**
     * @param WindowResponse|null $window
     *
     * @return self
     */
    public function setWindow(?WindowResponse $window): self
    {
        $this->initialized['window'] = true;
        $this->window = $window;
        return $this;
    }
}