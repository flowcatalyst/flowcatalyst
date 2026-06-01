<?php

namespace FlowCatalyst\Generated\Model;

class BatchCreateDispatchJobsResponse extends \ArrayObject
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
    protected $count;
    /**
     * @var list<DispatchJobResponse>|null
     */
    protected $jobs;
    /**
     * @return int|null
     */
    public function getCount(): ?int
    {
        return $this->count;
    }
    /**
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
     * @return list<DispatchJobResponse>|null
     */
    public function getJobs(): ?array
    {
        return $this->jobs;
    }
    /**
     * @param list<DispatchJobResponse>|null $jobs
     *
     * @return self
     */
    public function setJobs(?array $jobs): self
    {
        $this->initialized['jobs'] = true;
        $this->jobs = $jobs;
        return $this;
    }
}