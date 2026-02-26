<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminPlatformCorsAllowedGetResponse200 extends \ArrayObject
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
    protected $origins;
    /**
     * @return list<string>|null
     */
    public function getOrigins(): ?array
    {
        return $this->origins;
    }
    /**
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