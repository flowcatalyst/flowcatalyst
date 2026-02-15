<?php

namespace FlowCatalyst\Generated\Model;

class ApplicationResponse extends \ArrayObject
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
     * Application type
     *
     * @var string|null
     */
    protected $type;
    /**
     * Unique application code
     *
     * @var string|null
     */
    protected $code;
    /**
     * Display name
     *
     * @var string|null
     */
    protected $name;
    /**
     * Description
     *
     * @var string|null
     */
    protected $description;
    /**
     * Default base URL
     *
     * @var string|null
     */
    protected $defaultBaseUrl;
    /**
     * Icon URL
     *
     * @var string|null
     */
    protected $iconUrl;
    /**
     * Website URL
     *
     * @var string|null
     */
    protected $website;
    /**
     * Logo data (base64)
     *
     * @var string|null
     */
    protected $logo;
    /**
     * Logo MIME type
     *
     * @var string|null
     */
    protected $logoMimeType;
    /**
     * Service account OAuth client ID
     *
     * @var string|null
     */
    protected $serviceAccountId;
    /**
     * Service account principal ID
     *
     * @var string|null
     */
    protected $serviceAccountPrincipalId;
    /**
     * Whether application is active
     *
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
     * Service account information
     *
     * @var ServiceAccountInfo|null
     */
    protected $serviceAccount;
    /**
     * Warning message if any
     *
     * @var string|null
     */
    protected $warning;
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
     * Application type
     *
     * @return string|null
     */
    public function getType(): ?string
    {
        return $this->type;
    }
    /**
     * Application type
     *
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
     * Unique application code
     *
     * @return string|null
     */
    public function getCode(): ?string
    {
        return $this->code;
    }
    /**
     * Unique application code
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
     * Display name
     *
     * @return string|null
     */
    public function getName(): ?string
    {
        return $this->name;
    }
    /**
     * Display name
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
     * Description
     *
     * @return string|null
     */
    public function getDescription(): ?string
    {
        return $this->description;
    }
    /**
     * Description
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
     * Default base URL
     *
     * @return string|null
     */
    public function getDefaultBaseUrl(): ?string
    {
        return $this->defaultBaseUrl;
    }
    /**
     * Default base URL
     *
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
     * Website URL
     *
     * @return string|null
     */
    public function getWebsite(): ?string
    {
        return $this->website;
    }
    /**
     * Website URL
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
     * Logo data (base64)
     *
     * @return string|null
     */
    public function getLogo(): ?string
    {
        return $this->logo;
    }
    /**
     * Logo data (base64)
     *
     * @param string|null $logo
     *
     * @return self
     */
    public function setLogo(?string $logo): self
    {
        $this->initialized['logo'] = true;
        $this->logo = $logo;
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
     * Service account OAuth client ID
     *
     * @return string|null
     */
    public function getServiceAccountId(): ?string
    {
        return $this->serviceAccountId;
    }
    /**
     * Service account OAuth client ID
     *
     * @param string|null $serviceAccountId
     *
     * @return self
     */
    public function setServiceAccountId(?string $serviceAccountId): self
    {
        $this->initialized['serviceAccountId'] = true;
        $this->serviceAccountId = $serviceAccountId;
        return $this;
    }
    /**
     * Service account principal ID
     *
     * @return string|null
     */
    public function getServiceAccountPrincipalId(): ?string
    {
        return $this->serviceAccountPrincipalId;
    }
    /**
     * Service account principal ID
     *
     * @param string|null $serviceAccountPrincipalId
     *
     * @return self
     */
    public function setServiceAccountPrincipalId(?string $serviceAccountPrincipalId): self
    {
        $this->initialized['serviceAccountPrincipalId'] = true;
        $this->serviceAccountPrincipalId = $serviceAccountPrincipalId;
        return $this;
    }
    /**
     * Whether application is active
     *
     * @return bool|null
     */
    public function getActive(): ?bool
    {
        return $this->active;
    }
    /**
     * Whether application is active
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
    /**
     * Service account information
     *
     * @return ServiceAccountInfo|null
     */
    public function getServiceAccount(): ?ServiceAccountInfo
    {
        return $this->serviceAccount;
    }
    /**
     * Service account information
     *
     * @param ServiceAccountInfo|null $serviceAccount
     *
     * @return self
     */
    public function setServiceAccount(?ServiceAccountInfo $serviceAccount): self
    {
        $this->initialized['serviceAccount'] = true;
        $this->serviceAccount = $serviceAccount;
        return $this;
    }
    /**
     * Warning message if any
     *
     * @return string|null
     */
    public function getWarning(): ?string
    {
        return $this->warning;
    }
    /**
     * Warning message if any
     *
     * @param string|null $warning
     *
     * @return self
     */
    public function setWarning(?string $warning): self
    {
        $this->initialized['warning'] = true;
        $this->warning = $warning;
        return $this;
    }
}