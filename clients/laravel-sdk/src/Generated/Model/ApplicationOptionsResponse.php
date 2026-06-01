<?php

namespace FlowCatalyst\Generated\Model;

class ApplicationOptionsResponse extends \ArrayObject
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
     * @var list<ApplicationOption>|null
     */
    protected $options;
    /**
     * @return list<ApplicationOption>|null
     */
    public function getOptions(): ?array
    {
        return $this->options;
    }
    /**
     * @param list<ApplicationOption>|null $options
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