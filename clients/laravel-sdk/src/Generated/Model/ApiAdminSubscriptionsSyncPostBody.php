<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminSubscriptionsSyncPostBody extends \ArrayObject
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
     * @var string|null
     */
    protected $applicationCode;
    /**
     * @var list<ApiAdminSubscriptionsSyncPostBodySubscriptionsItem>|null
     */
    protected $subscriptions;
    /**
     * @var bool|null
     */
    protected $removeUnlisted;
    /**
     * @return string|null
     */
    public function getApplicationCode(): ?string
    {
        return $this->applicationCode;
    }
    /**
     * @param string|null $applicationCode
     *
     * @return self
     */
    public function setApplicationCode(?string $applicationCode): self
    {
        $this->initialized['applicationCode'] = true;
        $this->applicationCode = $applicationCode;
        return $this;
    }
    /**
     * @return list<ApiAdminSubscriptionsSyncPostBodySubscriptionsItem>|null
     */
    public function getSubscriptions(): ?array
    {
        return $this->subscriptions;
    }
    /**
     * @param list<ApiAdminSubscriptionsSyncPostBodySubscriptionsItem>|null $subscriptions
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
     * @return bool|null
     */
    public function getRemoveUnlisted(): ?bool
    {
        return $this->removeUnlisted;
    }
    /**
     * @param bool|null $removeUnlisted
     *
     * @return self
     */
    public function setRemoveUnlisted(?bool $removeUnlisted): self
    {
        $this->initialized['removeUnlisted'] = true;
        $this->removeUnlisted = $removeUnlisted;
        return $this;
    }
}