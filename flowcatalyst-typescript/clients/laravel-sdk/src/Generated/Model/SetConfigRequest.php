<?php

namespace FlowCatalyst\Generated\Model;

class SetConfigRequest extends \ArrayObject
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
    protected $value;
    /**
     * @var string|null
     */
    protected $valueType;
    /**
     * @var string|null
     */
    protected $description;
    /**
     * @return string|null
     */
    public function getValue(): ?string
    {
        return $this->value;
    }
    /**
     * @param string|null $value
     *
     * @return self
     */
    public function setValue(?string $value): self
    {
        $this->initialized['value'] = true;
        $this->value = $value;
        return $this;
    }
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
    /**
     * @return string|null
     */
    public function getDescription(): ?string
    {
        return $this->description;
    }
    /**
     * @param string|null $description
     *
     * @return self
     */
    public function setDescription(?string $description): self
    {
        $this->initialized['description'] = true;
        $this->description = $description;
        return $this;
    }
}