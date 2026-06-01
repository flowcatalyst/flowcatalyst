<?php

namespace FlowCatalyst\Generated\Model;

class RegisterCompleteRequest extends \ArrayObject
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
     * The `PublicKeyCredential` returned by `navigator.credentials.create()`.
     *
     * @var array<string, mixed>|null
     */
    protected $credential;
    /**
     * User-supplied label (e.g. "Andrew's iPhone").
     *
     * @var string|null
     */
    protected $name;
    /**
     * @var string|null
     */
    protected $stateId;
    /**
     * The `PublicKeyCredential` returned by `navigator.credentials.create()`.
     *
     * @return array<string, mixed>|null
     */
    public function getCredential(): ?iterable
    {
        return $this->credential;
    }
    /**
     * The `PublicKeyCredential` returned by `navigator.credentials.create()`.
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
     * User-supplied label (e.g. "Andrew's iPhone").
     *
     * @return string|null
     */
    public function getName(): ?string
    {
        return $this->name;
    }
    /**
     * User-supplied label (e.g. "Andrew's iPhone").
     *
     * @param string|null $name
     *
     * @return self
     */
    public function setName(?string $name): self
    {
        $this->initialized['name'] = true;
        $this->name = $name;
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