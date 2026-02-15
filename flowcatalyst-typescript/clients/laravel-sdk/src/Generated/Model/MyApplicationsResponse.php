<?php

namespace FlowCatalyst\Generated\Model;

class MyApplicationsResponse extends \ArrayObject
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
     * Applications enabled for this client
     *
     * @var list<MyApplicationDto>|null
     */
    protected $applications;
    /**
     * Total count
     *
     * @var int|null
     */
    protected $total;
    /**
     * Client ID these applications belong to
     *
     * @var string|null
     */
    protected $clientId;
    /**
     * Applications enabled for this client
     *
     * @return list<MyApplicationDto>|null
     */
    public function getApplications(): ?array
    {
        return $this->applications;
    }
    /**
     * Applications enabled for this client
     *
     * @param list<MyApplicationDto>|null $applications
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
    /**
     * Client ID these applications belong to
     *
     * @return string|null
     */
    public function getClientId(): ?string
    {
        return $this->clientId;
    }
    /**
     * Client ID these applications belong to
     *
     * @param string|null $clientId
     *
     * @return self
     */
    public function setClientId(?string $clientId): self
    {
        $this->initialized['clientId'] = true;
        $this->clientId = $clientId;
        return $this;
    }
}