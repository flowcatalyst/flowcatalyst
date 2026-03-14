<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminPrincipalsIdClientAccessGetResponse200 extends \ArrayObject
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
     * @var list<ApiAdminPrincipalsIdClientAccessGetResponse200GrantsItem>|null
     */
    protected $grants;
    /**
     * @return list<ApiAdminPrincipalsIdClientAccessGetResponse200GrantsItem>|null
     */
    public function getGrants(): ?array
    {
        return $this->grants;
    }
    /**
     * @param list<ApiAdminPrincipalsIdClientAccessGetResponse200GrantsItem>|null $grants
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