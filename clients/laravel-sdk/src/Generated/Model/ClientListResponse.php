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
     * @var list<ClientResponse>|null
     */
    protected $clients;
    /**
     * @var int|null
     */
    protected $total;
    /**
     * @return list<ClientResponse>|null
     */
    public function getClients(): ?array
    {
        return $this->clients;
    }
    /**
     * @param list<ClientResponse>|null $clients
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