<?php

namespace FlowCatalyst\Generated\Model;

class PlatformStats extends \ArrayObject
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
     * @var int|null
     */
    protected $eventTypes;
    /**
     * @var int|null
     */
    protected $subscriptions;
    /**
     * @var int|null
     */
    protected $messagesProcessed;
    /**
     * @return int|null
     */
    public function getEventTypes(): ?int
    {
        return $this->eventTypes;
    }
    /**
     * @param int|null $eventTypes
     *
     * @return self
     */
    public function setEventTypes(?int $eventTypes): self
    {
        $this->initialized['eventTypes'] = true;
        $this->eventTypes = $eventTypes;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getSubscriptions(): ?int
    {
        return $this->subscriptions;
    }
    /**
     * @param int|null $subscriptions
     *
     * @return self
     */
    public function setSubscriptions(?int $subscriptions): self
    {
        $this->initialized['subscriptions'] = true;
        $this->subscriptions = $subscriptions;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getMessagesProcessed(): ?int
    {
        return $this->messagesProcessed;
    }
    /**
     * @param int|null $messagesProcessed
     *
     * @return self
     */
    public function setMessagesProcessed(?int $messagesProcessed): self
    {
        $this->initialized['messagesProcessed'] = true;
        $this->messagesProcessed = $messagesProcessed;
        return $this;
    }
}