<?php

namespace FlowCatalyst\Generated\Model;

class ClientAccessListResponse extends \ArrayObject
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
     * @var list<ClientAccessGrantResponse>|null
     */
    protected $grants;
    /**
     * @return list<ClientAccessGrantResponse>|null
     */
    public function getGrants(): ?array
    {
        return $this->grants;
    }
    /**
     * @param list<ClientAccessGrantResponse>|null $grants
     *
     * @return self
     */
    public function setGrants(?array $grants): self
    {
        $this->initialized['grants'] = true;
        $this->grants = $grants;
        return $this;
    }
}