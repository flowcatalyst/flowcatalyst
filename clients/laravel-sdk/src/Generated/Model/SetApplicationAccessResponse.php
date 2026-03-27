<?php

namespace FlowCatalyst\Generated\Model;

class SetApplicationAccessResponse extends \ArrayObject
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
    protected $added;
    /**
     * @var list<ApplicationAccessResponse>|null
     */
    protected $applications;
    /**
     * @var int|null
     */
    protected $removed;
    /**
     * @return int|null
     */
    public function getAdded(): ?int
    {
        return $this->added;
    }
    /**
     * @param int|null $added
     *
     * @return self
     */
    public function setAdded(?int $added): self
    {
        $this->initialized['added'] = true;
        $this->added = $added;
        return $this;
    }
    /**
     * @return list<ApplicationAccessResponse>|null
     */
    public function getApplications(): ?array
    {
        return $this->applications;
    }
    /**
     * @param list<ApplicationAccessResponse>|null $applications
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
     * @return int|null
     */
    public function getRemoved(): ?int
    {
        return $this->removed;
    }
    /**
     * @param int|null $removed
     *
     * @return self
     */
    public function setRemoved(?int $removed): self
    {
        $this->initialized['removed'] = true;
        $this->removed = $removed;
        return $this;
    }
}