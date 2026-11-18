<?php

namespace FlowCatalyst\Generated\Model;

class AppUpdateResponse
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
     * The manifest's createdAt or the rollback's commitTime
     *
     * @var \DateTime|null
     */
    protected $createdAt;
    /**
     * @var string|null
     */
    protected $id;
    /**
     * @var string|null
     */
    protected $kind;
    /**
     * servedPercent > 0
     *
     * @var bool|null
     */
    protected $live;
    /**
     * The manifest's UUID (updates only)
     *
     * @var string|null
     */
    protected $manifestId;
    /**
     * @var string|null
     */
    protected $message;
    /**
     * @var string|null
     */
    protected $platform;
    /**
     * The principal that published it
     *
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
    protected $republishedFrom;
    /**
     * @var int|null
     */
    protected $rolloutPercent;
    /**
     * @var string|null
     */
    protected $runtimeVersion;
    /**
     * Share of the branch's phones this row is served to now: the newest row its rollout, the one before it the rest while the newest is partial, any other 0
     *
     * @var int|null
     */
    protected $servedPercent;
    /**
     * @var string|null
     */
    protected $sourceCommit;
    /**
     * @var \DateTime|null
     */
    protected $updatedAt;
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
     * The manifest's createdAt or the rollback's commitTime
     *
     * @return \DateTime|null
     */
    public function getCreatedAt(): ?\DateTime
    {
        return $this->createdAt;
    }
    /**
     * The manifest's createdAt or the rollback's commitTime
     *
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
    public function getKind(): ?string
    {
        return $this->kind;
    }
    /**
     * @param string|null $kind
     *
     * @return self
     */
    public function setKind(?string $kind): self
    {
        $this->initialized['kind'] = true;
        $this->kind = $kind;
        return $this;
    }
    /**
     * servedPercent > 0
     *
     * @return bool|null
     */
    public function getLive(): ?bool
    {
        return $this->live;
    }
    /**
     * servedPercent > 0
     *
     * @param bool|null $live
     *
     * @return self
     */
    public function setLive(?bool $live): self
    {
        $this->initialized['live'] = true;
        $this->live = $live;
        return $this;
    }
    /**
     * The manifest's UUID (updates only)
     *
     * @return string|null
     */
    public function getManifestId(): ?string
    {
        return $this->manifestId;
    }
    /**
     * The manifest's UUID (updates only)
     *
     * @param string|null $manifestId
     *
     * @return self
     */
    public function setManifestId(?string $manifestId): self
    {
        $this->initialized['manifestId'] = true;
        $this->manifestId = $manifestId;
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
     * @return string|null
     */
    public function getPlatform(): ?string
    {
        return $this->platform;
    }
    /**
     * @param string|null $platform
     *
     * @return self
     */
    public function setPlatform(?string $platform): self
    {
        $this->initialized['platform'] = true;
        $this->platform = $platform;
        return $this;
    }
    /**
     * The principal that published it
     *
     * @return string|null
     */
    public function getPublishedBy(): ?string
    {
        return $this->publishedBy;
    }
    /**
     * The principal that published it
     *
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
    public function getRepublishedFrom(): ?string
    {
        return $this->republishedFrom;
    }
    /**
     * @param string|null $republishedFrom
     *
     * @return self
     */
    public function setRepublishedFrom(?string $republishedFrom): self
    {
        $this->initialized['republishedFrom'] = true;
        $this->republishedFrom = $republishedFrom;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getRolloutPercent(): ?int
    {
        return $this->rolloutPercent;
    }
    /**
     * @param int|null $rolloutPercent
     *
     * @return self
     */
    public function setRolloutPercent(?int $rolloutPercent): self
    {
        $this->initialized['rolloutPercent'] = true;
        $this->rolloutPercent = $rolloutPercent;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getRuntimeVersion(): ?string
    {
        return $this->runtimeVersion;
    }
    /**
     * @param string|null $runtimeVersion
     *
     * @return self
     */
    public function setRuntimeVersion(?string $runtimeVersion): self
    {
        $this->initialized['runtimeVersion'] = true;
        $this->runtimeVersion = $runtimeVersion;
        return $this;
    }
    /**
     * Share of the branch's phones this row is served to now: the newest row its rollout, the one before it the rest while the newest is partial, any other 0
     *
     * @return int|null
     */
    public function getServedPercent(): ?int
    {
        return $this->servedPercent;
    }
    /**
     * Share of the branch's phones this row is served to now: the newest row its rollout, the one before it the rest while the newest is partial, any other 0
     *
     * @param int|null $servedPercent
     *
     * @return self
     */
    public function setServedPercent(?int $servedPercent): self
    {
        $this->initialized['servedPercent'] = true;
        $this->servedPercent = $servedPercent;
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