<?php

namespace FlowCatalyst\Generated\Model;

class RegisterBeginRequest extends \ArrayObject
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
     * Display name shown in the authenticator UI (defaults to the user's name).
     *
     * @var string|null
     */
    protected $displayName;
    /**
     * Display name shown in the authenticator UI (defaults to the user's name).
     *
     * @return string|null
     */
    public function getDisplayName(): ?string
    {
        return $this->displayName;
    }
    /**
     * Display name shown in the authenticator UI (defaults to the user's name).
     *
     * @param string|null $displayName
     *
     * @return self
     */
    public function setDisplayName(?string $displayName): self
    {
        $this->initialized['displayName'] = true;
        $this->displayName = $displayName;
        return $this;
    }
}