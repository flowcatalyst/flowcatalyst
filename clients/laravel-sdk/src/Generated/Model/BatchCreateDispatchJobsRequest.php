<?php

namespace FlowCatalyst\Generated\Model;

class BatchCreateDispatchJobsRequest extends \ArrayObject
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
     * @var list<CreateDispatchJobRequest>|null
     */
    protected $jobs;
    /**
     * @return list<CreateDispatchJobRequest>|null
     */
    public function getJobs(): ?array
    {
        return $this->jobs;
    }
    /**
     * @param list<CreateDispatchJobRequest>|null $jobs
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