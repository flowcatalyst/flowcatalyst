<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminApplicationsPostResponse201 extends \ArrayObject
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
    protected $type;
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
     * @var mixed|null
     */
    protected $serviceAccountId;
    /**
     * @var bool|null
     */
    protected $active;
    /**
     * @var \DateTime|null
     */
    protected $createdAt;
    /**
     * @var \DateTime|null
     */
    protected $updatedAt;
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
    public function getType(): ?string
    {
        return $this->type;
    }
    /**
     * @param string|null $type
     *
     * @return self
     */
    public function setType(?string $type): self
    {
        $this->initialized['type'] = true;
        $this->type = $type;
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
    /**
     * @return mixed
     */
    public function getServiceAccountId()
    {
        return $this->serviceAccountId;
    }
    /**
     * @param mixed $serviceAccountId
     *
     * @return self
     */
    public function setServiceAccountId($serviceAccountId): self
    {
        $this->initialized['serviceAccountId'] = true;
        $this->serviceAccountId = $serviceAccountId;
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
     * @return \DateTime|null
     */
    public function getCreatedAt(): ?\DateTime
    {
        return $this->createdAt;
    }
    /**
     * @param \DateTime|null $createdAt
     *
     * @return self
     */
    public function setCreatedAt(?\DateTime $createdAt): self
    {
        $this->initialized['createdAt'] = true;
        $this->createdAt = $createdAt;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getUpdatedAt(): ?\DateTime
    {
        return $this->updatedAt;
    }
    /**
     * @param \DateTime|null $updatedAt
     *
     * @return self
     */
    public function setUpdatedAt(?\DateTime $updatedAt): self
    {
        $this->initialized['updatedAt'] = true;
        $this->updatedAt = $updatedAt;
        return $this;
    }
}