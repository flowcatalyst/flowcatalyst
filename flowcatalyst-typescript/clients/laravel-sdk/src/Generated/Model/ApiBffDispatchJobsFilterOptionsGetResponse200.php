<?php

namespace FlowCatalyst\Generated\Model;

class ApiBffDispatchJobsFilterOptionsGetResponse200 extends \ArrayObject
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
     * @var list<string>|null
     */
    protected $applications;
    /**
     * @var list<string>|null
     */
    protected $subdomains;
    /**
     * @var list<string>|null
     */
    protected $aggregates;
    /**
     * @var list<string>|null
     */
    protected $codes;
    /**
     * @var list<string>|null
     */
    protected $statuses;
    /**
     * @return list<string>|null
     */
    public function getApplications(): ?array
    {
        return $this->applications;
    }
    /**
     * @param list<string>|null $applications
     *
     * @return self
     */
    public function setApplications(?array $applications): self
    {
        $this->initialized['applications'] = true;
        $this->applications = $applications;
        return $this;
    }
    /**
     * @return list<string>|null
     */
    public function getSubdomains(): ?array
    {
        return $this->subdomains;
    }
    /**
     * @param list<string>|null $subdomains
     *
     * @return self
     */
    public function setSubdomains(?array $subdomains): self
    {
        $this->initialized['subdomains'] = true;
        $this->subdomains = $subdomains;
        return $this;
    }
    /**
     * @return list<string>|null
     */
    public function getAggregates(): ?array
    {
        return $this->aggregates;
    }
    /**
     * @param list<string>|null $aggregates
     *
     * @return self
     */
    public function setAggregates(?array $aggregates): self
    {
        $this->initialized['aggregates'] = true;
        $this->aggregates = $aggregates;
        return $this;
    }
    /**
     * @return list<string>|null
     */
    public function getCodes(): ?array
    {
        return $this->codes;
    }
    /**
     * @param list<string>|null $codes
     *
     * @return self
     */
    public function setCodes(?array $codes): self
    {
        $this->initialized['codes'] = true;
        $this->codes = $codes;
        return $this;
    }
    /**
     * @return list<string>|null
     */
    public function getStatuses(): ?array
    {
        return $this->statuses;
    }
    /**
     * @param list<string>|null $statuses
     *
     * @return self
     */
    public function setStatuses(?array $statuses): self
    {
        $this->initialized['statuses'] = true;
        $this->statuses = $statuses;
        return $this;
    }
}