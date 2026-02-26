<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminClientsIdApplicationsGetResponse200 extends \ArrayObject
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
     * @var list<ApiAdminClientsIdApplicationsGetResponse200ApplicationsItem>|null
     */
    protected $applications;
    /**
     * @var int|null
     */
    protected $total;
    /**
     * @return list<ApiAdminClientsIdApplicationsGetResponse200ApplicationsItem>|null
     */
    public function getApplications(): ?array
    {
        return $this->applications;
    }
    /**
     * @param list<ApiAdminClientsIdApplicationsGetResponse200ApplicationsItem>|null $applications
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
    public function getTotal(): ?int
    {
        return $this->total;
    }
    /**
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