<?php

namespace FlowCatalyst\Generated\Model;

class MyClientsResponse extends \ArrayObject
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
     * Clients the user can access
     *
     * @var list<MyClientDto>|null
     */
    protected $clients;
    /**
     * Total count
     *
     * @var int|null
     */
    protected $total;
    /**
     * Clients the user can access
     *
     * @return list<MyClientDto>|null
     */
    public function getClients(): ?array
    {
        return $this->clients;
    }
    /**
     * Clients the user can access
     *
     * @param list<MyClientDto>|null $clients
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