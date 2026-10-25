<?php

namespace FlowCatalyst\Generated\Model;

class IngestionResponse
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
    protected $applicationCode;
    /**
     * @var string|null
     */
    protected $applicationId;
    /**
     * @var string|null
     */
    protected $clientId;
    /**
     * @var string|null
     */
    protected $clientIdentifier;
    /**
     * @var string|null
     */
    protected $code;
    /**
     * @var \DateTime|null
     */
    protected $createdAt;
    /**
     * @var string|null
     */
    protected $id;
    /**
     * Keep the first 2 KB of a payload that failed. Off unless the client has agreed.
     *
     * @var bool|null
     */
    protected $keepFailedPayloads;
    /**
     * The newest undismissed notice's message
     *
     * @var string|null
     */
    protected $latestNotice;
    /**
     * @var \DateTime|null
     */
    protected $latestNoticeAt;
    /**
     * @var int|null
     */
    protected $latestVersion;
    /**
     * Id of the set that owns the ingestion; it is read-only until the set is released
     *
     * @var string|null
     */
    protected $managedBy;
    /**
     * @var string|null
     */
    protected $name;
    /**
     * Undismissed notices: rejected failures (GET .../failures?category=rejected&undismissed=true)
     *
     * @var int|null
     */
    protected $notices;
    /**
     * @var string|null
     */
    protected $runtimeNotice;
    /**
     * @var IngestionSaveOutcome|null
     */
    protected $save;
    /**
     * Where the data comes from: {kind: http|sftp|s3|sqs|rabbitmq|database-poll|api-poll|custom, ...}
     *
     * @var mixed|null
     */
    protected $source;
    /**
     * How a payload becomes records: {kind: none|csv|json|xml, ...}
     *
     * @var mixed|null
     */
    protected $split;
    /**
     * @var string|null
     */
    protected $status;
    /**
     * @var Target|null
     */
    protected $target;
    /**
     * @var \DateTime|null
     */
    protected $updatedAt;
    /**
     * @var int|null
     */
    protected $version;
    /**
     * What the definition says that will not run as meant, such as an sftp source that neither names the server's host key nor accepts any
     *
     * @var list<string>|null
     */
    protected $warnings;
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
    public function getClientId(): ?string
    {
        return $this->clientId;
    }
    /**
     * @param string|null $clientId
     *
     * @return self
     */
    public function setClientId(?string $clientId): self
    {
        $this->initialized['clientId'] = true;
        $this->clientId = $clientId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getClientIdentifier(): ?string
    {
        return $this->clientIdentifier;
    }
    /**
     * @param string|null $clientIdentifier
     *
     * @return self
     */
    public function setClientIdentifier(?string $clientIdentifier): self
    {
        $this->initialized['clientIdentifier'] = true;
        $this->clientIdentifier = $clientIdentifier;
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
     * Keep the first 2 KB of a payload that failed. Off unless the client has agreed.
     *
     * @return bool|null
     */
    public function getKeepFailedPayloads(): ?bool
    {
        return $this->keepFailedPayloads;
    }
    /**
     * Keep the first 2 KB of a payload that failed. Off unless the client has agreed.
     *
     * @param bool|null $keepFailedPayloads
     *
     * @return self
     */
    public function setKeepFailedPayloads(?bool $keepFailedPayloads): self
    {
        $this->initialized['keepFailedPayloads'] = true;
        $this->keepFailedPayloads = $keepFailedPayloads;
        return $this;
    }
    /**
     * The newest undismissed notice's message
     *
     * @return string|null
     */
    public function getLatestNotice(): ?string
    {
        return $this->latestNotice;
    }
    /**
     * The newest undismissed notice's message
     *
     * @param string|null $latestNotice
     *
     * @return self
     */
    public function setLatestNotice(?string $latestNotice): self
    {
        $this->initialized['latestNotice'] = true;
        $this->latestNotice = $latestNotice;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getLatestNoticeAt(): ?\DateTime
    {
        return $this->latestNoticeAt;
    }
    /**
     * @param \DateTime|null $latestNoticeAt
     *
     * @return self
     */
    public function setLatestNoticeAt(?\DateTime $latestNoticeAt): self
    {
        $this->initialized['latestNoticeAt'] = true;
        $this->latestNoticeAt = $latestNoticeAt;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getLatestVersion(): ?int
    {
        return $this->latestVersion;
    }
    /**
     * @param int|null $latestVersion
     *
     * @return self
     */
    public function setLatestVersion(?int $latestVersion): self
    {
        $this->initialized['latestVersion'] = true;
        $this->latestVersion = $latestVersion;
        return $this;
    }
    /**
     * Id of the set that owns the ingestion; it is read-only until the set is released
     *
     * @return string|null
     */
    public function getManagedBy(): ?string
    {
        return $this->managedBy;
    }
    /**
     * Id of the set that owns the ingestion; it is read-only until the set is released
     *
     * @param string|null $managedBy
     *
     * @return self
     */
    public function setManagedBy(?string $managedBy): self
    {
        $this->initialized['managedBy'] = true;
        $this->managedBy = $managedBy;
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
     * Undismissed notices: rejected failures (GET .../failures?category=rejected&undismissed=true)
     *
     * @return int|null
     */
    public function getNotices(): ?int
    {
        return $this->notices;
    }
    /**
     * Undismissed notices: rejected failures (GET .../failures?category=rejected&undismissed=true)
     *
     * @param int|null $notices
     *
     * @return self
     */
    public function setNotices(?int $notices): self
    {
        $this->initialized['notices'] = true;
        $this->notices = $notices;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getRuntimeNotice(): ?string
    {
        return $this->runtimeNotice;
    }
    /**
     * @param string|null $runtimeNotice
     *
     * @return self
     */
    public function setRuntimeNotice(?string $runtimeNotice): self
    {
        $this->initialized['runtimeNotice'] = true;
        $this->runtimeNotice = $runtimeNotice;
        return $this;
    }
    /**
     * @return IngestionSaveOutcome|null
     */
    public function getSave(): ?IngestionSaveOutcome
    {
        return $this->save;
    }
    /**
     * @param IngestionSaveOutcome|null $save
     *
     * @return self
     */
    public function setSave(?IngestionSaveOutcome $save): self
    {
        $this->initialized['save'] = true;
        $this->save = $save;
        return $this;
    }
    /**
     * Where the data comes from: {kind: http|sftp|s3|sqs|rabbitmq|database-poll|api-poll|custom, ...}
     *
     * @return mixed
     */
    public function getSource()
    {
        return $this->source;
    }
    /**
     * Where the data comes from: {kind: http|sftp|s3|sqs|rabbitmq|database-poll|api-poll|custom, ...}
     *
     * @param mixed $source
     *
     * @return self
     */
    public function setSource($source): self
    {
        $this->initialized['source'] = true;
        $this->source = $source;
        return $this;
    }
    /**
     * How a payload becomes records: {kind: none|csv|json|xml, ...}
     *
     * @return mixed
     */
    public function getSplit()
    {
        return $this->split;
    }
    /**
     * How a payload becomes records: {kind: none|csv|json|xml, ...}
     *
     * @param mixed $split
     *
     * @return self
     */
    public function setSplit($split): self
    {
        $this->initialized['split'] = true;
        $this->split = $split;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getStatus(): ?string
    {
        return $this->status;
    }
    /**
     * @param string|null $status
     *
     * @return self
     */
    public function setStatus(?string $status): self
    {
        $this->initialized['status'] = true;
        $this->status = $status;
        return $this;
    }
    /**
     * @return Target|null
     */
    public function getTarget(): ?Target
    {
        return $this->target;
    }
    /**
     * @param Target|null $target
     *
     * @return self
     */
    public function setTarget(?Target $target): self
    {
        $this->initialized['target'] = true;
        $this->target = $target;
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
     * @return int|null
     */
    public function getVersion(): ?int
    {
        return $this->version;
    }
    /**
     * @param int|null $version
     *
     * @return self
     */
    public function setVersion(?int $version): self
    {
        $this->initialized['version'] = true;
        $this->version = $version;
        return $this;
    }
    /**
     * What the definition says that will not run as meant, such as an sftp source that neither names the server's host key nor accepts any
     *
     * @return list<string>|null
     */
    public function getWarnings(): ?array
    {
        return $this->warnings;
    }
    /**
     * What the definition says that will not run as meant, such as an sftp source that neither names the server's host key nor accepts any
     *
     * @param list<string>|null $warnings
     *
     * @return self
     */
    public function setWarnings(?array $warnings): self
    {
        $this->initialized['warnings'] = true;
        $this->warnings = $warnings;
        return $this;
    }
}