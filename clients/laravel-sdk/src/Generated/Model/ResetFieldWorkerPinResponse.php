<?php

namespace FlowCatalyst\Generated\Model;

class ResetFieldWorkerPinResponse
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
     * @var bool|null
     */
    protected $pinChangeRequired;
    /**
     * @var string|null
     */
    protected $temporaryPin;
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
     * @return bool|null
     */
    public function getPinChangeRequired(): ?bool
    {
        return $this->pinChangeRequired;
    }
    /**
     * @param bool|null $pinChangeRequired
     *
     * @return self
     */
    public function setPinChangeRequired(?bool $pinChangeRequired): self
    {
        $this->initialized['pinChangeRequired'] = true;
        $this->pinChangeRequired = $pinChangeRequired;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getTemporaryPin(): ?string
    {
        return $this->temporaryPin;
    }
    /**
     * @param string|null $temporaryPin
     *
     * @return self
     */
    public function setTemporaryPin(?string $temporaryPin): self
    {
        $this->initialized['temporaryPin'] = true;
        $this->temporaryPin = $temporaryPin;
        return $this;
    }
}