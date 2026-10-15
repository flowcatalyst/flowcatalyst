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
     * Jobs the filter matches that the caller can see
     *
     * @var int|null
     */
    protected $count;
    /**
     * How many of them a requeue would reset for this caller; send it back as confirmCount
     *
     * @var int|null
     */
    protected $eligibleCount;
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
     * Jobs the filter matches that the caller can see
     *
     * @return int|null
     */
    public function getCount(): ?int
    {
        return $this->count;
    }
    /**
     * Jobs the filter matches that the caller can see
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
     * How many of them a requeue would reset for this caller; send it back as confirmCount
     *
     * @return int|null
     */
    public function getEligibleCount(): ?int
    {
        return $this->eligibleCount;
    }
    /**
     * How many of them a requeue would reset for this caller; send it back as confirmCount
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
}