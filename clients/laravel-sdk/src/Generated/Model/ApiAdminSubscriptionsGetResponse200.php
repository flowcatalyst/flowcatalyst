<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminSubscriptionsGetResponse200 extends \ArrayObject
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
     * @var list<ApiAdminSubscriptionsGetResponse200SubscriptionsItem>|null
     */
    protected $subscriptions;
    /**
     * @var int|null
     */
    protected $total;
    /**
     * @return list<ApiAdminSubscriptionsGetResponse200SubscriptionsItem>|null
     */
    public function getSubscriptions(): ?array
    {
        return $this->subscriptions;
    }
    /**
     * @param list<ApiAdminSubscriptionsGetResponse200SubscriptionsItem>|null $subscriptions
     *
     * @return self
     */
    public function setSubscriptions(?array $subscriptions): self
    {
        $this->initialized['subscriptions'] = true;
        $this->subscriptions = $subscriptions;
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