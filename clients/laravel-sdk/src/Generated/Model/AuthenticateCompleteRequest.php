<?php

namespace FlowCatalyst\Generated\Model;

class AuthenticateCompleteRequest extends \ArrayObject
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
     * The `PublicKeyCredential` returned by `navigator.credentials.get()`.
     *
     * @var array<string, mixed>|null
     */
    protected $credential;
    /**
     * @var string|null
     */
    protected $stateId;
    /**
     * The `PublicKeyCredential` returned by `navigator.credentials.get()`.
     *
     * @return array<string, mixed>|null
     */
    public function getCredential(): ?iterable
    {
        return $this->credential;
    }
    /**
     * The `PublicKeyCredential` returned by `navigator.credentials.get()`.
     *
     * @param array<string, mixed>|null $credential
     *
     * @return self
     */
    public function setCredential(?iterable $credential): self
    {
        $this->initialized['credential'] = true;
        $this->credential = $credential;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getStateId(): ?string
    {
        return $this->stateId;
    }
    /**
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