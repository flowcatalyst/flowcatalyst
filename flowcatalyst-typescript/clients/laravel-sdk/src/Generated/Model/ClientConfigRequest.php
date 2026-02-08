<?php

namespace FlowCatalyst\Generated\Model;

class ClientConfigRequest extends \ArrayObject
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
     * @var bool|null
     */
    protected $enabled;
    /**
     * @var string|null
     */
    protected $baseUrlOverride;
    /**
     * @var string|null
     */
    protected $websiteOverride;
    /**
     * @var array<string, mixed>|null
     */
    protected $config;
    /**
     * @return bool|null
     */
    public function getEnabled(): ?bool
    {
        return $this->enabled;
    }
    /**
     * @param bool|null $enabled
     *
     * @return self
     */
    public function setEnabled(?bool $enabled): self
    {
        $this->initialized['enabled'] = true;
        $this->enabled = $enabled;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getBaseUrlOverride(): ?string
    {
        return $this->baseUrlOverride;
    }
    /**
     * @param string|null $baseUrlOverride
     *
     * @return self
     */
    public function setBaseUrlOverride(?string $baseUrlOverride): self
    {
        $this->initialized['baseUrlOverride'] = true;
        $this->baseUrlOverride = $baseUrlOverride;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getWebsiteOverride(): ?string
    {
        return $this->websiteOverride;
    }
    /**
     * @param string|null $websiteOverride
     *
     * @return self
     */
    public function setWebsiteOverride(?string $websiteOverride): self
    {
        $this->initialized['websiteOverride'] = true;
        $this->websiteOverride = $websiteOverride;
        return $this;
    }
    /**
     * @return array<string, mixed>|null
     */
    public function getConfig(): ?iterable
    {
        return $this->config;
    }
    /**
     * @param array<string, mixed>|null $config
     *
     * @return self
     */
    public function setConfig(?iterable $config): self
    {
        $this->initialized['config'] = true;
        $this->config = $config;
        return $this;
    }
}