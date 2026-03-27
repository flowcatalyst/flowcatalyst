<?php

namespace FlowCatalyst\Generated\Model;

class SyncSubscriptionsRequest extends \ArrayObject
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
     * Application code
     *
     * @var string|null
     */
    protected $applicationCode;
    /**
     * Subscriptions to sync
     *
     * @var list<SyncSubscriptionInputRequest>|null
     */
    protected $subscriptions;
    /**
     * Application code
     *
     * @return string|null
     */
    public function getApplicationCode(): ?string
    {
        return $this->applicationCode;
    }
    /**
     * Application code
     *
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
     * Subscriptions to sync
     *
     * @return list<SyncSubscriptionInputRequest>|null
     */
    public function getSubscriptions(): ?array
    {
        return $this->subscriptions;
    }
    /**
     * Subscriptions to sync
     *
     * @param list<SyncSubscriptionInputRequest>|null $subscriptions
     *
     * @return self
     */
    public function setSubscriptions(?array $subscriptions): self
    {
        $this->initialized['subscriptions'] = true;
        $this->subscriptions = $subscriptions;
        return $this;
    }
}