<?php

namespace FlowCatalyst\Generated\Model;

class ApiSdkPrincipalsIdClientsGetResponse200 extends \ArrayObject
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
     * @var list<ApiSdkPrincipalsIdClientsGetResponse200GrantsItem>|null
     */
    protected $grants;
    /**
     * @return list<ApiSdkPrincipalsIdClientsGetResponse200GrantsItem>|null
     */
    public function getGrants(): ?array
    {
        return $this->grants;
    }
    /**
     * @param list<ApiSdkPrincipalsIdClientsGetResponse200GrantsItem>|null $grants
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