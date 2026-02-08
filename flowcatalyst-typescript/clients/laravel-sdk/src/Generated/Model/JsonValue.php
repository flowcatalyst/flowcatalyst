<?php

namespace FlowCatalyst\Generated\Model;

class JsonValue extends \ArrayObject
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
     * @var string|null
     */
    protected $valueType;
    /**
     * @return string|null
     */
    public function getValueType(): ?string
    {
        return $this->valueType;
    }
    /**
     * @param string|null $valueType
     *
     * @return self
     */
    public function setValueType(?string $valueType): self
    {
        $this->initialized['valueType'] = true;
        $this->valueType = $valueType;
        return $this;
    }
}