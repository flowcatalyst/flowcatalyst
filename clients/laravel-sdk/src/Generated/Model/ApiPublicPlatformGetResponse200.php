<?php

namespace FlowCatalyst\Generated\Model;

class ApiPublicPlatformGetResponse200 extends \ArrayObject
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
     * @var ApiPublicPlatformGetResponse200Features|null
     */
    protected $features;
    /**
     * @return ApiPublicPlatformGetResponse200Features|null
     */
    public function getFeatures(): ?ApiPublicPlatformGetResponse200Features
    {
        return $this->features;
    }
    /**
     * @param ApiPublicPlatformGetResponse200Features|null $features
     *
     * @return self
     */
    public function setFeatures(?ApiPublicPlatformGetResponse200Features $features): self
    {
        $this->initialized['features'] = true;
        $this->features = $features;
        return $this;
    }
}