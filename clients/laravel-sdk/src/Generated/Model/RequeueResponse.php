<?php

namespace FlowCatalyst\Generated\Model;

class RequeueResponse
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
     * How many of them were COMPLETED jobs re-delivered (0 unless the request redrove)
     *
     * @var int|null
     */
    protected $redriven;
    /**
     * Jobs reset to PENDING (redriven COMPLETED jobs included)
     *
     * @var int|null
     */
    protected $requeued;
    /**
     * The skipped jobs, with the reason
     *
     * @var list<SkippedJobResponse>|null
     */
    protected $skipped;
    /**
     * COMPLETED jobs left alone because the redrive was not requested and confirmed
     *
     * @var int|null
     */
    protected $skippedCompleted;
    /**
     * QUEUED or PROCESSING jobs left alone
     *
     * @var int|null
     */
    protected $skippedInFlight;
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
     * How many of them were COMPLETED jobs re-delivered (0 unless the request redrove)
     *
     * @return int|null
     */
    public function getRedriven(): ?int
    {
        return $this->redriven;
    }
    /**
     * How many of them were COMPLETED jobs re-delivered (0 unless the request redrove)
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
     * The skipped jobs, with the reason
     *
     * @return list<SkippedJobResponse>|null
     */
    public function getSkipped(): ?array
    {
        return $this->skipped;
    }
    /**
     * The skipped jobs, with the reason
     *
     * @param list<SkippedJobResponse>|null $skipped
     *
     * @return self
     */
    public function setSkipped(?array $skipped): self
    {
        $this->initialized['skipped'] = true;
        $this->skipped = $skipped;
        return $this;
    }
    /**
     * COMPLETED jobs left alone because the redrive was not requested and confirmed
     *
     * @return int|null
     */
    public function getSkippedCompleted(): ?int
    {
        return $this->skippedCompleted;
    }
    /**
     * COMPLETED jobs left alone because the redrive was not requested and confirmed
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
     * QUEUED or PROCESSING jobs left alone
     *
     * @return int|null
     */
    public function getSkippedInFlight(): ?int
    {
        return $this->skippedInFlight;
    }
    /**
     * QUEUED or PROCESSING jobs left alone
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
}