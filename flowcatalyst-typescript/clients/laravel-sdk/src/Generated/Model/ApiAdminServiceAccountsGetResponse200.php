<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminServiceAccountsGetResponse200 extends \ArrayObject
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
     * @var list<ApiAdminServiceAccountsGetResponse200ServiceAccountsItem>|null
     */
    protected $serviceAccounts;
    /**
     * @var int|null
     */
    protected $total;
    /**
     * @return list<ApiAdminServiceAccountsGetResponse200ServiceAccountsItem>|null
     */
    public function getServiceAccounts(): ?array
    {
        return $this->serviceAccounts;
    }
    /**
     * @param list<ApiAdminServiceAccountsGetResponse200ServiceAccountsItem>|null $serviceAccounts
     *
     * @return self
     */
    public function setServiceAccounts(?array $serviceAccounts): self
    {
        $this->initialized['serviceAccounts'] = true;
        $this->serviceAccounts = $serviceAccounts;
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