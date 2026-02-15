<?php

namespace FlowCatalyst\Generated\Model;

class ClientApplicationsResponse extends \ArrayObject
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
     * List of applications with their enabled status
     *
     * @var list<ClientApplicationDto>|null
     */
    protected $applications;
    /**
     * Total count
     *
     * @var int|null
     */
    protected $total;
    /**
     * List of applications with their enabled status
     *
     * @return list<ClientApplicationDto>|null
     */
    public function getApplications(): ?array
    {
        return $this->applications;
    }
    /**
     * List of applications with their enabled status
     *
     * @param list<ClientApplicationDto>|null $applications
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