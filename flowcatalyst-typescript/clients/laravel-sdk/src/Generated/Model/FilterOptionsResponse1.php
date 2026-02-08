<?php

namespace FlowCatalyst\Generated\Model;

class FilterOptionsResponse1 extends \ArrayObject
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
     * @var list<FilterOption>|null
     */
    protected $clients;
    /**
     * @var list<FilterOption>|null
     */
    protected $applications;
    /**
     * @var list<FilterOption>|null
     */
    protected $subdomains;
    /**
     * @var list<FilterOption>|null
     */
    protected $aggregates;
    /**
     * @var list<FilterOption>|null
     */
    protected $codes;
    /**
     * @var list<FilterOption>|null
     */
    protected $statuses;
    /**
     * @return list<FilterOption>|null
     */
    public function getClients(): ?array
    {
        return $this->clients;
    }
    /**
     * @param list<FilterOption>|null $clients
     *
     * @return self
     */
    public function setClients(?array $clients): self
    {
        $this->initialized['clients'] = true;
        $this->clients = $clients;
        return $this;
    }
    /**
     * @return list<FilterOption>|null
     */
    public function getApplications(): ?array
    {
        return $this->applications;
    }
    /**
     * @param list<FilterOption>|null $applications
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
     * @return list<FilterOption>|null
     */
    public function getSubdomains(): ?array
    {
        return $this->subdomains;
    }
    /**
     * @param list<FilterOption>|null $subdomains
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
     * @return list<FilterOption>|null
     */
    public function getAggregates(): ?array
    {
        return $this->aggregates;
    }
    /**
     * @param list<FilterOption>|null $aggregates
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
     * @return list<FilterOption>|null
     */
    public function getCodes(): ?array
    {
        return $this->codes;
    }
    /**
     * @param list<FilterOption>|null $codes
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
     * @return list<FilterOption>|null
     */
    public function getStatuses(): ?array
    {
        return $this->statuses;
    }
    /**
     * @param list<FilterOption>|null $statuses
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