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
     * Client access grants
     *
     * @var list<ClientAccessGrantDto>|null
     */
    protected $grants;
    /**
     * Client access grants
     *
     * @return list<ClientAccessGrantDto>|null
     */
    public function getGrants(): ?array
    {
        return $this->grants;
    }
    /**
     * Client access grants
     *
     * @param list<ClientAccessGrantDto>|null $grants
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