<?php

namespace FlowCatalyst\Generated\Model;

class ExchangeWindow extends \ArrayObject
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
     * @var int|null
     */
    protected $completed;
    /**
     * @var int|null
     */
    protected $dispatchJobs;
    /**
     * @var int|null
     */
    protected $failed;
    /**
     * @var \DateTime|null
     */
    protected $from;
    /**
     * @var string|null
     */
    protected $ingestionId;
    /**
     * @var int|null
     */
    protected $records;
    /**
     * @var \DateTime|null
     */
    protected $to;
    /**
     * @return int|null
     */
    public function getCompleted(): ?int
    {
        return $this->completed;
    }
    /**
     * @param int|null $completed
     *
     * @return self
     */
    public function setCompleted(?int $completed): self
    {
        $this->initialized['completed'] = true;
        $this->completed = $completed;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getDispatchJobs(): ?int
    {
        return $this->dispatchJobs;
    }
    /**
     * @param int|null $dispatchJobs
     *
     * @return self
     */
    public function setDispatchJobs(?int $dispatchJobs): self
    {
        $this->initialized['dispatchJobs'] = true;
        $this->dispatchJobs = $dispatchJobs;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getFailed(): ?int
    {
        return $this->failed;
    }
    /**
     * @param int|null $failed
     *
     * @return self
     */
    public function setFailed(?int $failed): self
    {
        $this->initialized['failed'] = true;
        $this->failed = $failed;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getFrom(): ?\DateTime
    {
        return $this->from;
    }
    /**
     * @param \DateTime|null $from
     *
     * @return self
     */
    public function setFrom(?\DateTime $from): self
    {
        $this->initialized['from'] = true;
        $this->from = $from;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getIngestionId(): ?string
    {
        return $this->ingestionId;
    }
    /**
     * @param string|null $ingestionId
     *
     * @return self
     */
    public function setIngestionId(?string $ingestionId): self
    {
        $this->initialized['ingestionId'] = true;
        $this->ingestionId = $ingestionId;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getRecords(): ?int
    {
        return $this->records;
    }
    /**
     * @param int|null $records
     *
     * @return self
     */
    public function setRecords(?int $records): self
    {
        $this->initialized['records'] = true;
        $this->records = $records;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getTo(): ?\DateTime
    {
        return $this->to;
    }
    /**
     * @param \DateTime|null $to
     *
     * @return self
     */
    public function setTo(?\DateTime $to): self
    {
        $this->initialized['to'] = true;
        $this->to = $to;
        return $this;
    }
}