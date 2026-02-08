<?php

namespace FlowCatalyst\Generated\Model;

class ClientApplicationDto extends \ArrayObject
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
     * @var string|null
     */
    protected $description;
    /**
     * @var string|null
     */
    protected $iconUrl;
    /**
     * @var string|null
     */
    protected $website;
    /**
     * @var string|null
     */
    protected $effectiveWebsite;
    /**
     * @var string|null
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
    /**
     * @return string|null
     */
    public function getWebsite(): ?string
    {
        return $this->website;
    }
    /**
     * @param string|null $website
     *
     * @return self
     */
    public function setWebsite(?string $website): self
    {
        $this->initialized['website'] = true;
        $this->website = $website;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getEffectiveWebsite(): ?string
    {
        return $this->effectiveWebsite;
    }
    /**
     * @param string|null $effectiveWebsite
     *
     * @return self
     */
    public function setEffectiveWebsite(?string $effectiveWebsite): self
    {
        $this->initialized['effectiveWebsite'] = true;
        $this->effectiveWebsite = $effectiveWebsite;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getLogoMimeType(): ?string
    {
        return $this->logoMimeType;
    }
    /**
     * @param string|null $logoMimeType
     *
     * @return self
     */
    public function setLogoMimeType(?string $logoMimeType): self
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