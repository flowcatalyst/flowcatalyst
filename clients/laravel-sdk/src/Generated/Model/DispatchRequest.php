<?php

namespace FlowCatalyst\Generated\Model;

class DispatchRequest extends \ArrayObject
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
     * A URL to the JSON Schema for this object.
     *
     * @var string|null
     */
    protected $dollarSchema;
    /**
     * @var Dispatch|null
     */
    protected $dispatch;
    /**
     * @var string|null
     */
    protected $functionId;
    /**
     * @var string|null
     */
    protected $invocationId;
    /**
     * @var string|null
     */
    protected $payload;
    /**
     * @var string|null
     */
    protected $requestCorrelationId;
    /**
     * @var int|null
     */
    protected $version;
    /**
     * A URL to the JSON Schema for this object.
     *
     * @return string|null
     */
    public function getDollarSchema(): ?string
    {
        return $this->dollarSchema;
    }
    /**
     * A URL to the JSON Schema for this object.
     *
     * @param string|null $dollarSchema
     *
     * @return self
     */
    public function setDollarSchema(?string $dollarSchema): self
    {
        $this->initialized['dollarSchema'] = true;
        $this->dollarSchema = $dollarSchema;
        return $this;
    }
    /**
     * @return Dispatch|null
     */
    public function getDispatch(): ?Dispatch
    {
        return $this->dispatch;
    }
    /**
     * @param Dispatch|null $dispatch
     *
     * @return self
     */
    public function setDispatch(?Dispatch $dispatch): self
    {
        $this->initialized['dispatch'] = true;
        $this->dispatch = $dispatch;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getFunctionId(): ?string
    {
        return $this->functionId;
    }
    /**
     * @param string|null $functionId
     *
     * @return self
     */
    public function setFunctionId(?string $functionId): self
    {
        $this->initialized['functionId'] = true;
        $this->functionId = $functionId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getInvocationId(): ?string
    {
        return $this->invocationId;
    }
    /**
     * @param string|null $invocationId
     *
     * @return self
     */
    public function setInvocationId(?string $invocationId): self
    {
        $this->initialized['invocationId'] = true;
        $this->invocationId = $invocationId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getPayload(): ?string
    {
        return $this->payload;
    }
    /**
     * @param string|null $payload
     *
     * @return self
     */
    public function setPayload(?string $payload): self
    {
        $this->initialized['payload'] = true;
        $this->payload = $payload;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getRequestCorrelationId(): ?string
    {
        return $this->requestCorrelationId;
    }
    /**
     * @param string|null $requestCorrelationId
     *
     * @return self
     */
    public function setRequestCorrelationId(?string $requestCorrelationId): self
    {
        $this->initialized['requestCorrelationId'] = true;
        $this->requestCorrelationId = $requestCorrelationId;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getVersion(): ?int
    {
        return $this->version;
    }
    /**
     * @param int|null $version
     *
     * @return self
     */
    public function setVersion(?int $version): self
    {
        $this->initialized['version'] = true;
        $this->version = $version;
        return $this;
    }
}