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
     * Eligible jobs still matching after the run; run again to continue
     *
     * @var int|null
     */
    protected $remaining;
    /**
     * Jobs reset to PENDING
     *
     * @var int|null
     */
    protected $requeued;
    /**
     * The run stopped at its limit (or at confirmCount) with eligible jobs left
     *
     * @var bool|null
     */
    protected $truncated;
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
     * Jobs reset to PENDING
     *
     * @return int|null
     */
    public function getRequeued(): ?int
    {
        return $this->requeued;
    }
    /**
     * Jobs reset to PENDING
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
}