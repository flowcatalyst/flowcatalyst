<?php

namespace FlowCatalyst\Generated\Model;

class AccessibleClientsResponse extends \ArrayObject
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
     * @var list<ClientInfo>|null
     */
    protected $clients;
    /**
     * @var string|null
     */
    protected $currentClientId;
    /**
     * @var bool|null
     */
    protected $globalAccess;
    /**
     * @return list<ClientInfo>|null
     */
    public function getClients(): ?array
    {
        return $this->clients;
    }
    /**
     * @param list<ClientInfo>|null $clients
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
     * @return string|null
     */
    public function getCurrentClientId(): ?string
    {
        return $this->currentClientId;
    }
    /**
     * @param string|null $currentClientId
     *
     * @return self
     */
    public function setCurrentClientId(?string $currentClientId): self
    {
        $this->initialized['currentClientId'] = true;
        $this->currentClientId = $currentClientId;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getGlobalAccess(): ?bool
    {
        return $this->globalAccess;
    }
    /**
     * @param bool|null $globalAccess
     *
     * @return self
     */
    public function setGlobalAccess(?bool $globalAccess): self
    {
        $this->initialized['globalAccess'] = true;
        $this->globalAccess = $globalAccess;
        return $this;
    }
}