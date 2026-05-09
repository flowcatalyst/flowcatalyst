<?php

namespace FlowCatalyst\Generated\Model;

class OAuthClientListResponse extends \ArrayObject
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
     * @var list<OAuthClientResponse>|null
     */
    protected $clients;
    /**
     * @return list<OAuthClientResponse>|null
     */
    public function getClients(): ?array
    {
        return $this->clients;
    }
    /**
     * @param list<OAuthClientResponse>|null $clients
     *
     * @return self
     */
    public function setClients(?array $clients): self
    {
        $this->initialized['clients'] = true;
        $this->clients = $clients;
        return $this;
    }
}