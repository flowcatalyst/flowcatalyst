<?php

namespace FlowCatalyst\Generated\Model;

class ClientConfigListResponse extends \ArrayObject
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
     * Client configurations
     *
     * @var list<ClientConfigResponse>|null
     */
    protected $clientConfigs;
    /**
     * Total count
     *
     * @var int|null
     */
    protected $total;
    /**
     * Client configurations
     *
     * @return list<ClientConfigResponse>|null
     */
    public function getClientConfigs(): ?array
    {
        return $this->clientConfigs;
    }
    /**
     * Client configurations
     *
     * @param list<ClientConfigResponse>|null $clientConfigs
     *
     * @return self
     */
    public function setClientConfigs(?array $clientConfigs): self
    {
        $this->initialized['clientConfigs'] = true;
        $this->clientConfigs = $clientConfigs;
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