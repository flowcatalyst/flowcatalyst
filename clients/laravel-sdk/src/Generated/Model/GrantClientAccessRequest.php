<?php

namespace FlowCatalyst\Generated\Model;

class GrantClientAccessRequest extends \ArrayObject
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
     * Client ID to grant access to
     *
     * @var string|null
     */
    protected $clientId;
    /**
     * Client ID to grant access to
     *
     * @return string|null
     */
    public function getClientId(): ?string
    {
        return $this->clientId;
    }
    /**
     * Client ID to grant access to
     *
     * @param string|null $clientId
     *
     * @return self
     */
    public function setClientId(?string $clientId): self
    {
        $this->initialized['clientId'] = true;
        $this->clientId = $clientId;
        return $this;
    }
}