<?php

namespace FlowCatalyst\Generated\Model;

class ApiConfigPlatformGetResponse200 extends \ArrayObject
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
     * @var ApiConfigPlatformGetResponse200Features|null
     */
    protected $features;
    /**
     * @return ApiConfigPlatformGetResponse200Features|null
     */
    public function getFeatures(): ?ApiConfigPlatformGetResponse200Features
    {
        return $this->features;
    }
    /**
     * @param ApiConfigPlatformGetResponse200Features|null $features
     *
     * @return self
     */
    public function setFeatures(?ApiConfigPlatformGetResponse200Features $features): self
    {
        $this->initialized['features'] = true;
        $this->features = $features;
        return $this;
    }
}