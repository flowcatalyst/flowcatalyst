<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminIdentityProvidersGetResponse200 extends \ArrayObject
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
     * @var list<ApiAdminIdentityProvidersGetResponse200IdentityProvidersItem>|null
     */
    protected $identityProviders;
    /**
     * @var int|null
     */
    protected $total;
    /**
     * @return list<ApiAdminIdentityProvidersGetResponse200IdentityProvidersItem>|null
     */
    public function getIdentityProviders(): ?array
    {
        return $this->identityProviders;
    }
    /**
     * @param list<ApiAdminIdentityProvidersGetResponse200IdentityProvidersItem>|null $identityProviders
     *
     * @return self
     */
    public function setIdentityProviders(?array $identityProviders): self
    {
        $this->initialized['identityProviders'] = true;
        $this->identityProviders = $identityProviders;
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