<?php

namespace FlowCatalyst\Generated\Model;

class FilterOptionsResponse2 extends \ArrayObject
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
     * @var list<FilterOption1>|null
     */
    protected $clients;
    /**
     * @var list<FilterOption1>|null
     */
    protected $applications;
    /**
     * @var list<FilterOption1>|null
     */
    protected $subdomains;
    /**
     * @var list<FilterOption1>|null
     */
    protected $aggregates;
    /**
     * @var list<FilterOption1>|null
     */
    protected $types;
    /**
     * @return list<FilterOption1>|null
     */
    public function getClients(): ?array
    {
        return $this->clients;
    }
    /**
     * @param list<FilterOption1>|null $clients
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
     * @return list<FilterOption1>|null
     */
    public function getApplications(): ?array
    {
        return $this->applications;
    }
    /**
     * @param list<FilterOption1>|null $applications
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
     * @return list<FilterOption1>|null
     */
    public function getSubdomains(): ?array
    {
        return $this->subdomains;
    }
    /**
     * @param list<FilterOption1>|null $subdomains
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
     * @return list<FilterOption1>|null
     */
    public function getAggregates(): ?array
    {
        return $this->aggregates;
    }
    /**
     * @param list<FilterOption1>|null $aggregates
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
     * @return list<FilterOption1>|null
     */
    public function getTypes(): ?array
    {
        return $this->types;
    }
    /**
     * @param list<FilterOption1>|null $types
     *
     * @return self
     */
    public function setTypes(?array $types): self
    {
        $this->initialized['types'] = true;
        $this->types = $types;
        return $this;
    }
}