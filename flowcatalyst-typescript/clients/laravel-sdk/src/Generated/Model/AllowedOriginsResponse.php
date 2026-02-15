<?php

namespace FlowCatalyst\Generated\Model;

class AllowedOriginsResponse extends \ArrayObject
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
     * Set of allowed origin URLs
     *
     * @var list<string>|null
     */
    protected $origins;
    /**
     * Set of allowed origin URLs
     *
     * @return list<string>|null
     */
    public function getOrigins(): ?array
    {
        return $this->origins;
    }
    /**
     * Set of allowed origin URLs
     *
     * @param list<string>|null $origins
     *
     * @return self
     */
    public function setOrigins(?array $origins): self
    {
        $this->initialized['origins'] = true;
        $this->origins = $origins;
        return $this;
    }
}