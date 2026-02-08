<?php

namespace FlowCatalyst\Generated\Model;

class UpdateApplicationRequest1 extends \ArrayObject
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
    protected $name;
    /**
     * @var string|null
     */
    protected $description;
    /**
     * @var string|null
     */
    protected $defaultBaseUrl;
    /**
     * @var string|null
     */
    protected $iconUrl;
    /**
     * @return string|null
     */
    public function getName(): ?string
    {
        return $this->name;
    }
    /**
     * @param string|null $name
     *
     * @return self
     */
    public function setName(?string $name): self
    {
        $this->initialized['name'] = true;
        $this->name = $name;
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
    /**
     * @return string|null
     */
    public function getDefaultBaseUrl(): ?string
    {
        return $this->defaultBaseUrl;
    }
    /**
     * @param string|null $defaultBaseUrl
     *
     * @return self
     */
    public function setDefaultBaseUrl(?string $defaultBaseUrl): self
    {
        $this->initialized['defaultBaseUrl'] = true;
        $this->defaultBaseUrl = $defaultBaseUrl;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getIconUrl(): ?string
    {
        return $this->iconUrl;
    }
    /**
     * @param string|null $iconUrl
     *
     * @return self
     */
    public function setIconUrl(?string $iconUrl): self
    {
        $this->initialized['iconUrl'] = true;
        $this->iconUrl = $iconUrl;
        return $this;
    }
}