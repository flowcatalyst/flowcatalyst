<?php

namespace FlowCatalyst\Generated\Model;

class PublishLaunchAssetRequest
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
     * Lower-case hex SHA-256 of the stored bundle (the expo export metadata's bundle)
     *
     * @var string|null
     */
    protected $sha256;
    /**
     * Lower-case hex SHA-256 of the stored bundle (the expo export metadata's bundle)
     *
     * @return string|null
     */
    public function getSha256(): ?string
    {
        return $this->sha256;
    }
    /**
     * Lower-case hex SHA-256 of the stored bundle (the expo export metadata's bundle)
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