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
     * Application ID
     *
     * @var string|null
     */
    protected $id;
    /**
     * Application code
     *
     * @var string|null
     */
    protected $code;
    /**
     * Application name
     *
     * @var string|null
     */
    protected $name;
    /**
     * Application description
     *
     * @var string|null
     */
    protected $description;
    /**
     * Icon URL
     *
     * @var string|null
     */
    protected $iconUrl;
    /**
     * Default website URL
     *
     * @var string|null
     */
    protected $website;
    /**
     * Effective website URL (override or default)
     *
     * @var string|null
     */
    protected $effectiveWebsite;
    /**
     * Logo MIME type
     *
     * @var string|null
     */
    protected $logoMimeType;
    /**
     * Whether application is globally active
     *
     * @var bool|null
     */
    protected $active;
    /**
     * Whether application is enabled for this client
     *
     * @var bool|null
     */
    protected $enabledForClient;
    /**
     * Application ID
     *
     * @return string|null
     */
    public function getId(): ?string
    {
        return $this->id;
    }
    /**
     * Application ID
     *
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
     * Application code
     *
     * @return string|null
     */
    public function getCode(): ?string
    {
        return $this->code;
    }
    /**
     * Application code
     *
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
     * Application name
     *
     * @return string|null
     */
    public function getName(): ?string
    {
        return $this->name;
    }
    /**
     * Application name
     *
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
     * Application description
     *
     * @return string|null
     */
    public function getDescription(): ?string
    {
        return $this->description;
    }
    /**
     * Application description
     *
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
     * Icon URL
     *
     * @return string|null
     */
    public function getIconUrl(): ?string
    {
        return $this->iconUrl;
    }
    /**
     * Icon URL
     *
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
     * Default website URL
     *
     * @return string|null
     */
    public function getWebsite(): ?string
    {
        return $this->website;
    }
    /**
     * Default website URL
     *
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
     * Effective website URL (override or default)
     *
     * @return string|null
     */
    public function getEffectiveWebsite(): ?string
    {
        return $this->effectiveWebsite;
    }
    /**
     * Effective website URL (override or default)
     *
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
     * Logo MIME type
     *
     * @return string|null
     */
    public function getLogoMimeType(): ?string
    {
        return $this->logoMimeType;
    }
    /**
     * Logo MIME type
     *
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
     * Whether application is globally active
     *
     * @return bool|null
     */
    public function getActive(): ?bool
    {
        return $this->active;
    }
    /**
     * Whether application is globally active
     *
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
     * Whether application is enabled for this client
     *
     * @return bool|null
     */
    public function getEnabledForClient(): ?bool
    {
        return $this->enabledForClient;
    }
    /**
     * Whether application is enabled for this client
     *
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