<?php

namespace FlowCatalyst\Generated\Model;

class FileStoredResponse
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
     * A URL to the JSON Schema for this object.
     *
     * @var string|null
     */
    protected $dollarSchema;
    /**
     * false when the application had already stored this file
     *
     * @var bool|null
     */
    protected $created;
    /**
     * @var string|null
     */
    protected $sha256;
    /**
     * @var int|null
     */
    protected $sizeBytes;
    /**
     * A URL to the JSON Schema for this object.
     *
     * @return string|null
     */
    public function getDollarSchema(): ?string
    {
        return $this->dollarSchema;
    }
    /**
     * A URL to the JSON Schema for this object.
     *
     * @param string|null $dollarSchema
     *
     * @return self
     */
    public function setDollarSchema(?string $dollarSchema): self
    {
        $this->initialized['dollarSchema'] = true;
        $this->dollarSchema = $dollarSchema;
        return $this;
    }
    /**
     * false when the application had already stored this file
     *
     * @return bool|null
     */
    public function getCreated(): ?bool
    {
        return $this->created;
    }
    /**
     * false when the application had already stored this file
     *
     * @param bool|null $created
     *
     * @return self
     */
    public function setCreated(?bool $created): self
    {
        $this->initialized['created'] = true;
        $this->created = $created;
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
    /**
     * @return int|null
     */
    public function getSizeBytes(): ?int
    {
        return $this->sizeBytes;
    }
    /**
     * @param int|null $sizeBytes
     *
     * @return self
     */
    public function setSizeBytes(?int $sizeBytes): self
    {
        $this->initialized['sizeBytes'] = true;
        $this->sizeBytes = $sizeBytes;
        return $this;
    }
}