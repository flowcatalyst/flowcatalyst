<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminClientsIdApplicationsGetResponse200ApplicationsItem extends \ArrayObject
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
    protected $id;
    /**
     * @var string|null
     */
    protected $code;
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
    protected $logoMimeType;
    /**
     * @var bool|null
     */
    protected $active;
    /**
     * @var bool|null
     */
    protected $enabledForClient;
    /**
     * @return string|null
     */
    public function getId(): ?string
    {
        return $this->id;
    }
    /**
     * @param string|null $id
     *
     * @return self
     */
    public function setId(?string $id): self
    {
        $this->initialized['id'] = true;
        $this->id = $id;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getCode(): ?string
    {
        return $this->code;
    }
    /**
     * @param string|null $code
     *
     * @return self
     */
    public function setCode(?string $code): self
    {
        $this->initialized['code'] = true;
        $this->code = $code;
        return $this;
    }
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
     * @return bool|null
     */
    public function getActive(): ?bool
    {
        return $this->active;
    }
    /**
     * @param bool|null $active
     *
     * @return self
     */
    public function setActive(?bool $active): self
    {
        $this->initialized['active'] = true;
        $this->active = $active;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getEnabledForClient(): ?bool
    {
        return $this->enabledForClient;
    }
    /**
     * @param bool|null $enabledForClient
     *
     * @return self
     */
    public function setEnabledForClient(?bool $enabledForClient): self
    {
        $this->initialized['enabledForClient'] = true;
        $this->enabledForClient = $enabledForClient;
        return $this;
    }
}