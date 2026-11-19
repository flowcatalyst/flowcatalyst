<?php

namespace FlowCatalyst\Generated\Model;

class AppBinaryResponse
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
     * @var string|null
     */
    protected $appKey;
    /**
     * @var string|null
     */
    protected $applicationCode;
    /**
     * @var string|null
     */
    protected $applicationId;
    /**
     * @var string|null
     */
    protected $channel;
    /**
     * @var \DateTime|null
     */
    protected $createdAt;
    /**
     * @var AppBinaryFactsResponse|null
     */
    protected $facts;
    /**
     * The name it downloads under
     *
     * @var string|null
     */
    protected $fileName;
    /**
     * @var string|null
     */
    protected $id;
    /**
     * @var string|null
     */
    protected $message;
    /**
     * The newest (highest version code) binary of its channel and variant: what an install link gives
     *
     * @var bool|null
     */
    protected $newest;
    /**
     * @var string|null
     */
    protected $publishedBy;
    /**
     * @var string|null
     */
    protected $publishedByName;
    /**
     * @var string|null
     */
    protected $sha256;
    /**
     * @var int|null
     */
    protected $sizeBytes;
    /**
     * @var string|null
     */
    protected $sourceCommit;
    /**
     * @var string|null
     */
    protected $variant;
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
     * @return string|null
     */
    public function getAppKey(): ?string
    {
        return $this->appKey;
    }
    /**
     * @param string|null $appKey
     *
     * @return self
     */
    public function setAppKey(?string $appKey): self
    {
        $this->initialized['appKey'] = true;
        $this->appKey = $appKey;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getApplicationCode(): ?string
    {
        return $this->applicationCode;
    }
    /**
     * @param string|null $applicationCode
     *
     * @return self
     */
    public function setApplicationCode(?string $applicationCode): self
    {
        $this->initialized['applicationCode'] = true;
        $this->applicationCode = $applicationCode;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getApplicationId(): ?string
    {
        return $this->applicationId;
    }
    /**
     * @param string|null $applicationId
     *
     * @return self
     */
    public function setApplicationId(?string $applicationId): self
    {
        $this->initialized['applicationId'] = true;
        $this->applicationId = $applicationId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getChannel(): ?string
    {
        return $this->channel;
    }
    /**
     * @param string|null $channel
     *
     * @return self
     */
    public function setChannel(?string $channel): self
    {
        $this->initialized['channel'] = true;
        $this->channel = $channel;
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
     * @return AppBinaryFactsResponse|null
     */
    public function getFacts(): ?AppBinaryFactsResponse
    {
        return $this->facts;
    }
    /**
     * @param AppBinaryFactsResponse|null $facts
     *
     * @return self
     */
    public function setFacts(?AppBinaryFactsResponse $facts): self
    {
        $this->initialized['facts'] = true;
        $this->facts = $facts;
        return $this;
    }
    /**
     * The name it downloads under
     *
     * @return string|null
     */
    public function getFileName(): ?string
    {
        return $this->fileName;
    }
    /**
     * The name it downloads under
     *
     * @param string|null $fileName
     *
     * @return self
     */
    public function setFileName(?string $fileName): self
    {
        $this->initialized['fileName'] = true;
        $this->fileName = $fileName;
        return $this;
    }
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
    public function getMessage(): ?string
    {
        return $this->message;
    }
    /**
     * @param string|null $message
     *
     * @return self
     */
    public function setMessage(?string $message): self
    {
        $this->initialized['message'] = true;
        $this->message = $message;
        return $this;
    }
    /**
     * The newest (highest version code) binary of its channel and variant: what an install link gives
     *
     * @return bool|null
     */
    public function getNewest(): ?bool
    {
        return $this->newest;
    }
    /**
     * The newest (highest version code) binary of its channel and variant: what an install link gives
     *
     * @param bool|null $newest
     *
     * @return self
     */
    public function setNewest(?bool $newest): self
    {
        $this->initialized['newest'] = true;
        $this->newest = $newest;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getPublishedBy(): ?string
    {
        return $this->publishedBy;
    }
    /**
     * @param string|null $publishedBy
     *
     * @return self
     */
    public function setPublishedBy(?string $publishedBy): self
    {
        $this->initialized['publishedBy'] = true;
        $this->publishedBy = $publishedBy;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getPublishedByName(): ?string
    {
        return $this->publishedByName;
    }
    /**
     * @param string|null $publishedByName
     *
     * @return self
     */
    public function setPublishedByName(?string $publishedByName): self
    {
        $this->initialized['publishedByName'] = true;
        $this->publishedByName = $publishedByName;
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
    /**
     * @return string|null
     */
    public function getSourceCommit(): ?string
    {
        return $this->sourceCommit;
    }
    /**
     * @param string|null $sourceCommit
     *
     * @return self
     */
    public function setSourceCommit(?string $sourceCommit): self
    {
        $this->initialized['sourceCommit'] = true;
        $this->sourceCommit = $sourceCommit;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getVariant(): ?string
    {
        return $this->variant;
    }
    /**
     * @param string|null $variant
     *
     * @return self
     */
    public function setVariant(?string $variant): self
    {
        $this->initialized['variant'] = true;
        $this->variant = $variant;
        return $this;
    }
}