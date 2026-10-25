<?php

namespace FlowCatalyst\Generated\Model;

class IngestionFailureResponse
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
     * @var int|null
     */
    protected $afterRecords;
    /**
     * payload: a record or payload the runtime could not take. rejected: a notice, the platform refused the runtime's jobs permanently
     *
     * @var string|null
     */
    protected $category;
    /**
     * @var \DateTime|null
     */
    protected $createdAt;
    /**
     * @var \DateTime|null
     */
    protected $dismissedAt;
    /**
     * The principal who dismissed it
     *
     * @var string|null
     */
    protected $dismissedBy;
    /**
     * The start of the payload, only for an ingestion that keeps failed payloads
     *
     * @var string|null
     */
    protected $excerpt;
    /**
     * @var string|null
     */
    protected $id;
    /**
     * @var string|null
     */
    protected $kind;
    /**
     * @var string|null
     */
    protected $message;
    /**
     * @var \DateTime|null
     */
    protected $occurredAt;
    /**
     * @var string|null
     */
    protected $runtimeId;
    /**
     * @var int|null
     */
    protected $version;
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
     * @return int|null
     */
    public function getAfterRecords(): ?int
    {
        return $this->afterRecords;
    }
    /**
     * @param int|null $afterRecords
     *
     * @return self
     */
    public function setAfterRecords(?int $afterRecords): self
    {
        $this->initialized['afterRecords'] = true;
        $this->afterRecords = $afterRecords;
        return $this;
    }
    /**
     * payload: a record or payload the runtime could not take. rejected: a notice, the platform refused the runtime's jobs permanently
     *
     * @return string|null
     */
    public function getCategory(): ?string
    {
        return $this->category;
    }
    /**
     * payload: a record or payload the runtime could not take. rejected: a notice, the platform refused the runtime's jobs permanently
     *
     * @param string|null $category
     *
     * @return self
     */
    public function setCategory(?string $category): self
    {
        $this->initialized['category'] = true;
        $this->category = $category;
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
    public function getDismissedAt(): ?\DateTime
    {
        return $this->dismissedAt;
    }
    /**
     * @param \DateTime|null $dismissedAt
     *
     * @return self
     */
    public function setDismissedAt(?\DateTime $dismissedAt): self
    {
        $this->initialized['dismissedAt'] = true;
        $this->dismissedAt = $dismissedAt;
        return $this;
    }
    /**
     * The principal who dismissed it
     *
     * @return string|null
     */
    public function getDismissedBy(): ?string
    {
        return $this->dismissedBy;
    }
    /**
     * The principal who dismissed it
     *
     * @param string|null $dismissedBy
     *
     * @return self
     */
    public function setDismissedBy(?string $dismissedBy): self
    {
        $this->initialized['dismissedBy'] = true;
        $this->dismissedBy = $dismissedBy;
        return $this;
    }
    /**
     * The start of the payload, only for an ingestion that keeps failed payloads
     *
     * @return string|null
     */
    public function getExcerpt(): ?string
    {
        return $this->excerpt;
    }
    /**
     * The start of the payload, only for an ingestion that keeps failed payloads
     *
     * @param string|null $excerpt
     *
     * @return self
     */
    public function setExcerpt(?string $excerpt): self
    {
        $this->initialized['excerpt'] = true;
        $this->excerpt = $excerpt;
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
     * @return \DateTime|null
     */
    public function getOccurredAt(): ?\DateTime
    {
        return $this->occurredAt;
    }
    /**
     * @param \DateTime|null $occurredAt
     *
     * @return self
     */
    public function setOccurredAt(?\DateTime $occurredAt): self
    {
        $this->initialized['occurredAt'] = true;
        $this->occurredAt = $occurredAt;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getRuntimeId(): ?string
    {
        return $this->runtimeId;
    }
    /**
     * @param string|null $runtimeId
     *
     * @return self
     */
    public function setRuntimeId(?string $runtimeId): self
    {
        $this->initialized['runtimeId'] = true;
        $this->runtimeId = $runtimeId;
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
}