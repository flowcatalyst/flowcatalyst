<?php

namespace FlowCatalyst\Generated\Model;

class PlatformConfigResponse extends \ArrayObject
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
     * @var FeaturesConfig|null
     */
    protected $features;
    /**
     * @return FeaturesConfig|null
     */
    public function getFeatures(): ?FeaturesConfig
    {
        return $this->features;
    }
    /**
     * @param FeaturesConfig|null $features
     *
     * @return self
     */
    public function setFeatures(?FeaturesConfig $features): self
    {
        $this->initialized['features'] = true;
        $this->features = $features;
        return $this;
    }
}