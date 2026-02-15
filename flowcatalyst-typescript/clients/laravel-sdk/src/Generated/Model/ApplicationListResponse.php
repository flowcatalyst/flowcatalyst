<?php

namespace FlowCatalyst\Generated\Model;

class ApplicationListResponse extends \ArrayObject
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
     * List of applications
     *
     * @var list<ApplicationListItem>|null
     */
    protected $applications;
    /**
     * Total count
     *
     * @var int|null
     */
    protected $total;
    /**
     * List of applications
     *
     * @return list<ApplicationListItem>|null
     */
    public function getApplications(): ?array
    {
        return $this->applications;
    }
    /**
     * List of applications
     *
     * @param list<ApplicationListItem>|null $applications
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
     * Total count
     *
     * @return int|null
     */
    public function getTotal(): ?int
    {
        return $this->total;
    }
    /**
     * Total count
     *
     * @param int|null $total
     *
     * @return self
     */
    public function setTotal(?int $total): self
    {
        $this->initialized['total'] = true;
        $this->total = $total;
        return $this;
    }
}