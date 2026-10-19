<?php

namespace FlowCatalyst\Generated\Model;

class RequeueByFilterResponse
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
     * How many of them were COMPLETED jobs re-delivered
     *
     * @var int|null
     */
    protected $redriven;
    /**
     * Eligible jobs still matching after the run; run again to continue
     *
     * @var int|null
     */
    protected $remaining;
    /**
     * Jobs reset to PENDING (redriven COMPLETED jobs included)
     *
     * @var int|null
     */
    protected $requeued;
    /**
     * COMPLETED matches left alone because the redrive was not requested and confirmed
     *
     * @var int|null
     */
    protected $skippedCompleted;
    /**
     * QUEUED or PROCESSING matches left alone (never reset)
     *
     * @var int|null
     */
    protected $skippedInFlight;
    /**
     * The run stopped at its limit (or at confirmCount) with eligible jobs left
     *
     * @var bool|null
     */
    protected $truncated;
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
     * How many of them were COMPLETED jobs re-delivered
     *
     * @return int|null
     */
    public function getRedriven(): ?int
    {
        return $this->redriven;
    }
    /**
     * How many of them were COMPLETED jobs re-delivered
     *
     * @param int|null $redriven
     *
     * @return self
     */
    public function setRedriven(?int $redriven): self
    {
        $this->initialized['redriven'] = true;
        $this->redriven = $redriven;
        return $this;
    }
    /**
     * Eligible jobs still matching after the run; run again to continue
     *
     * @return int|null
     */
    public function getRemaining(): ?int
    {
        return $this->remaining;
    }
    /**
     * Eligible jobs still matching after the run; run again to continue
     *
     * @param int|null $remaining
     *
     * @return self
     */
    public function setRemaining(?int $remaining): self
    {
        $this->initialized['remaining'] = true;
        $this->remaining = $remaining;
        return $this;
    }
    /**
     * Jobs reset to PENDING (redriven COMPLETED jobs included)
     *
     * @return int|null
     */
    public function getRequeued(): ?int
    {
        return $this->requeued;
    }
    /**
     * Jobs reset to PENDING (redriven COMPLETED jobs included)
     *
     * @param int|null $requeued
     *
     * @return self
     */
    public function setRequeued(?int $requeued): self
    {
        $this->initialized['requeued'] = true;
        $this->requeued = $requeued;
        return $this;
    }
    /**
     * COMPLETED matches left alone because the redrive was not requested and confirmed
     *
     * @return int|null
     */
    public function getSkippedCompleted(): ?int
    {
        return $this->skippedCompleted;
    }
    /**
     * COMPLETED matches left alone because the redrive was not requested and confirmed
     *
     * @param int|null $skippedCompleted
     *
     * @return self
     */
    public function setSkippedCompleted(?int $skippedCompleted): self
    {
        $this->initialized['skippedCompleted'] = true;
        $this->skippedCompleted = $skippedCompleted;
        return $this;
    }
    /**
     * QUEUED or PROCESSING matches left alone (never reset)
     *
     * @return int|null
     */
    public function getSkippedInFlight(): ?int
    {
        return $this->skippedInFlight;
    }
    /**
     * QUEUED or PROCESSING matches left alone (never reset)
     *
     * @param int|null $skippedInFlight
     *
     * @return self
     */
    public function setSkippedInFlight(?int $skippedInFlight): self
    {
        $this->initialized['skippedInFlight'] = true;
        $this->skippedInFlight = $skippedInFlight;
        return $this;
    }
    /**
     * The run stopped at its limit (or at confirmCount) with eligible jobs left
     *
     * @return bool|null
     */
    public function getTruncated(): ?bool
    {
        return $this->truncated;
    }
    /**
     * The run stopped at its limit (or at confirmCount) with eligible jobs left
     *
     * @param bool|null $truncated
     *
     * @return self
     */
    public function setTruncated(?bool $truncated): self
    {
        $this->initialized['truncated'] = true;
        $this->truncated = $truncated;
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