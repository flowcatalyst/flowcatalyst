<?php

namespace FlowCatalyst\Generated\Model;

class ApiPublicPlatformGetResponse200Features extends \ArrayObject
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
     * @var bool|null
     */
    protected $messagingEnabled;
    /**
     * @return bool|null
     */
    public function getMessagingEnabled(): ?bool
    {
        return $this->messagingEnabled;
    }
    /**
     * @param bool|null $messagingEnabled
     *
     * @return self
     */
    public function setMessagingEnabled(?bool $messagingEnabled): self
    {
        $this->initialized['messagingEnabled'] = true;
        $this->messagingEnabled = $messagingEnabled;
        return $this;
    }
}