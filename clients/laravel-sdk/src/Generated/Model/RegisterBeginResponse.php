<?php

namespace FlowCatalyst\Generated\Model;

class RegisterBeginResponse extends \ArrayObject
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
     * `PublicKeyCredentialCreationOptions` JSON for the browser.
     *
     * @var array<string, mixed>|null
     */
    protected $options;
    /**
     * Opaque ceremony state token; pass back unchanged on `register/complete`.
     *
     * @var string|null
     */
    protected $stateId;
    /**
     * `PublicKeyCredentialCreationOptions` JSON for the browser.
     *
     * @return array<string, mixed>|null
     */
    public function getOptions(): ?iterable
    {
        return $this->options;
    }
    /**
     * `PublicKeyCredentialCreationOptions` JSON for the browser.
     *
     * @param array<string, mixed>|null $options
     *
     * @return self
     */
    public function setOptions(?iterable $options): self
    {
        $this->initialized['options'] = true;
        $this->options = $options;
        return $this;
    }
    /**
     * Opaque ceremony state token; pass back unchanged on `register/complete`.
     *
     * @return string|null
     */
    public function getStateId(): ?string
    {
        return $this->stateId;
    }
    /**
     * Opaque ceremony state token; pass back unchanged on `register/complete`.
     *
     * @param string|null $stateId
     *
     * @return self
     */
    public function setStateId(?string $stateId): self
    {
        $this->initialized['stateId'] = true;
        $this->stateId = $stateId;
        return $this;
    }
}