<?php

namespace FlowCatalyst\Generated\Model;

class ClientApplicationResponse extends \ArrayObject
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
     * Whether the application itself is active globally
     *
     * @var bool|null
     */
    protected $active;
    /**
     * Application code
     *
     * @var string|null
     */
    protected $code;
    /**
     * Application description
     *
     * @var string|null
     */
    protected $description;
    /**
     * Whether this application is enabled for this specific client
     *
     * @var bool|null
     */
    protected $enabledForClient;
    /**
     * Application icon URL
     *
     * @var string|null
     */
    protected $iconUrl;
    /**
     * Application ID
     *
     * @var string|null
     */
    protected $id;
    /**
     * Application display name
     *
     * @var string|null
     */
    protected $name;
    /**
     * Whether the application itself is active globally
     *
     * @return bool|null
     */
    public function getActive(): ?bool
    {
        return $this->active;
    }
    /**
     * Whether the application itself is active globally
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
     * Whether this application is enabled for this specific client
     *
     * @return bool|null
     */
    public function getEnabledForClient(): ?bool
    {
        return $this->enabledForClient;
    }
    /**
     * Whether this application is enabled for this specific client
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
    /**
     * Application icon URL
     *
     * @return string|null
     */
    public function getIconUrl(): ?string
    {
        return $this->iconUrl;
    }
    /**
     * Application icon URL
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
     * Application display name
     *
     * @return string|null
     */
    public function getName(): ?string
    {
        return $this->name;
    }
    /**
     * Application display name
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
}