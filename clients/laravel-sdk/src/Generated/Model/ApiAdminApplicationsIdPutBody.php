<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminApplicationsIdPutBody extends \ArrayObject
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
     * @var mixed|null
     */
    protected $description;
    /**
     * @var mixed|null
     */
    protected $iconUrl;
    /**
     * @var mixed|null
     */
    protected $website;
    /**
     * @var mixed|null
     */
    protected $logo;
    /**
     * @var mixed|null
     */
    protected $logoMimeType;
    /**
     * @var mixed|null
     */
    protected $defaultBaseUrl;
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
     * @return mixed
     */
    public function getDescription()
    {
        return $this->description;
    }
    /**
     * @param mixed $description
     *
     * @return self
     */
    public function setDescription($description): self
    {
        $this->initialized['description'] = true;
        $this->description = $description;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getIconUrl()
    {
        return $this->iconUrl;
    }
    /**
     * @param mixed $iconUrl
     *
     * @return self
     */
    public function setIconUrl($iconUrl): self
    {
        $this->initialized['iconUrl'] = true;
        $this->iconUrl = $iconUrl;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getWebsite()
    {
        return $this->website;
    }
    /**
     * @param mixed $website
     *
     * @return self
     */
    public function setWebsite($website): self
    {
        $this->initialized['website'] = true;
        $this->website = $website;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getLogo()
    {
        return $this->logo;
    }
    /**
     * @param mixed $logo
     *
     * @return self
     */
    public function setLogo($logo): self
    {
        $this->initialized['logo'] = true;
        $this->logo = $logo;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getLogoMimeType()
    {
        return $this->logoMimeType;
    }
    /**
     * @param mixed $logoMimeType
     *
     * @return self
     */
    public function setLogoMimeType($logoMimeType): self
    {
        $this->initialized['logoMimeType'] = true;
        $this->logoMimeType = $logoMimeType;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getDefaultBaseUrl()
    {
        return $this->defaultBaseUrl;
    }
    /**
     * @param mixed $defaultBaseUrl
     *
     * @return self
     */
    public function setDefaultBaseUrl($defaultBaseUrl): self
    {
        $this->initialized['defaultBaseUrl'] = true;
        $this->defaultBaseUrl = $defaultBaseUrl;
        return $this;
    }
}