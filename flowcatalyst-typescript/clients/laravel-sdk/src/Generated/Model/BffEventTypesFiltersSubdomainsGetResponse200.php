<?php

namespace FlowCatalyst\Generated\Model;

class BffEventTypesFiltersSubdomainsGetResponse200 extends \ArrayObject
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
     * @var list<string>|null
     */
    protected $options;
    /**
     * @return list<string>|null
     */
    public function getOptions(): ?array
    {
        return $this->options;
    }
    /**
     * @param list<string>|null $options
     *
     * @return self
     */
    public function setOptions(?array $options): self
    {
        $this->initialized['options'] = true;
        $this->options = $options;
        return $this;
    }
}