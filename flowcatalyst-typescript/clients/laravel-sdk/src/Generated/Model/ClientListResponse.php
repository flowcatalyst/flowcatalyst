<?php

namespace FlowCatalyst\Generated\Model;

class ClientListResponse extends \ArrayObject
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
     * Client list
     *
     * @var list<ClientDto>|null
     */
    protected $clients;
    /**
     * Total count
     *
     * @var int|null
     */
    protected $total;
    /**
     * Client list
     *
     * @return list<ClientDto>|null
     */
    public function getClients(): ?array
    {
        return $this->clients;
    }
    /**
     * Client list
     *
     * @param list<ClientDto>|null $clients
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