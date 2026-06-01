<?php

namespace FlowCatalyst\Generated\Model;

class SubdomainsResponse extends \ArrayObject
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
     * @var list<FilterOption>|null
     */
    protected $subdomains;
    /**
     * @return list<FilterOption>|null
     */
    public function getSubdomains(): ?array
    {
        return $this->subdomains;
    }
    /**
     * @param list<FilterOption>|null $subdomains
     *
     * @return self
     */
    public function setSubdomains(?array $subdomains): self
    {
        $this->initialized['subdomains'] = true;
        $this->subdomains = $subdomains;
        return $this;
    }
}