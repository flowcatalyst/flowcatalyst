<?php

namespace FlowCatalyst\Generated\Model;

class PublishAssetRequest
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
     * The extension the phone saves it under (png or .png): the expo export metadata's ext
     *
     * @var string|null
     */
    protected $fileExtension;
    /**
     * Lower-case hex SHA-256 of a stored file
     *
     * @var string|null
     */
    protected $sha256;
    /**
     * The extension the phone saves it under (png or .png): the expo export metadata's ext
     *
     * @return string|null
     */
    public function getFileExtension(): ?string
    {
        return $this->fileExtension;
    }
    /**
     * The extension the phone saves it under (png or .png): the expo export metadata's ext
     *
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
     * Lower-case hex SHA-256 of a stored file
     *
     * @return string|null
     */
    public function getSha256(): ?string
    {
        return $this->sha256;
    }
    /**
     * Lower-case hex SHA-256 of a stored file
     *
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