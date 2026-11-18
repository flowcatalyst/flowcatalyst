<?php

namespace FlowCatalyst\Generated\Model;

class AppUpdateAssetResponse
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
    protected $contentType;
    /**
     * @var string|null
     */
    protected $fileExtension;
    /**
     * @var bool|null
     */
    protected $isLaunchAsset;
    /**
     * The manifest key (the file's MD5)
     *
     * @var string|null
     */
    protected $key;
    /**
     * @var string|null
     */
    protected $sha256;
    /**
     * @return string|null
     */
    public function getContentType(): ?string
    {
        return $this->contentType;
    }
    /**
     * @param string|null $contentType
     *
     * @return self
     */
    public function setContentType(?string $contentType): self
    {
        $this->initialized['contentType'] = true;
        $this->contentType = $contentType;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getFileExtension(): ?string
    {
        return $this->fileExtension;
    }
    /**
     * @param string|null $fileExtension
     *
     * @return self
     */
    public function setFileExtension(?string $fileExtension): self
    {
        $this->initialized['fileExtension'] = true;
        $this->fileExtension = $fileExtension;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getIsLaunchAsset(): ?bool
    {
        return $this->isLaunchAsset;
    }
    /**
     * @param bool|null $isLaunchAsset
     *
     * @return self
     */
    public function setIsLaunchAsset(?bool $isLaunchAsset): self
    {
        $this->initialized['isLaunchAsset'] = true;
        $this->isLaunchAsset = $isLaunchAsset;
        return $this;
    }
    /**
     * The manifest key (the file's MD5)
     *
     * @return string|null
     */
    public function getKey(): ?string
    {
        return $this->key;
    }
    /**
     * The manifest key (the file's MD5)
     *
     * @param string|null $key
     *
     * @return self
     */
    public function setKey(?string $key): self
    {
        $this->initialized['key'] = true;
        $this->key = $key;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getSha256(): ?string
    {
        return $this->sha256;
    }
    /**
     * @param string|null $sha256
     *
     * @return self
     */
    public function setSha256(?string $sha256): self
    {
        $this->initialized['sha256'] = true;
        $this->sha256 = $sha256;
        return $this;
    }
}