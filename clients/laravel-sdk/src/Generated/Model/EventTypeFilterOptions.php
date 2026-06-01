<?php

namespace FlowCatalyst\Generated\Model;

class EventTypeFilterOptions extends \ArrayObject
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
    protected $applications;
    /**
     * @var list<FilterOption>|null
     */
    protected $eventTypes;
    /**
     * @var list<FilterOption>|null
     */
    protected $subdomains;
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
    public function getEventTypes(): ?array
    {
        return $this->eventTypes;
    }
    /**
     * @param list<FilterOption>|null $eventTypes
     *
     * @return self
     */
    public function setEventTypes(?array $eventTypes): self
    {
        $this->initialized['eventTypes'] = true;
        $this->eventTypes = $eventTypes;
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
}