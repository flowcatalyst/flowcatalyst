<?php

namespace FlowCatalyst\Generated\Model;

class IngestionTestResponse
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
     * @var \DateTime|null
     */
    protected $completedAt;
    /**
     * @var \DateTime|null
     */
    protected $createdAt;
    /**
     * The start of the answer, only for an ingestion that keeps failed payloads
     *
     * @var string|null
     */
    protected $excerpt;
    /**
     * @var \DateTime|null
     */
    protected $expiresAt;
    /**
     * @var string|null
     */
    protected $id;
    /**
     * @var string|null
     */
    protected $ingestionId;
    /**
     * @var string|null
     */
    protected $kind;
    /**
     * Set while no live runtime carries the kind: the test waits, nothing has failed
     *
     * @var string|null
     */
    protected $notice;
    /**
     * @var string|null
     */
    protected $outcome;
    /**
     * How many records the split makes of the answer, when it answered
     *
     * @var int|null
     */
    protected $records;
    /**
     * @var string|null
     */
    protected $runtimeId;
    /**
     * PENDING until a runtime takes it, RUNNING while it tries, DONE with its result, EXPIRED when none answered in time
     *
     * @var string|null
     */
    protected $status;
    /**
     * What the runtime did, step by step, for a person
     *
     * @var list<string>|null
     */
    protected $steps;
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
     * @return \DateTime|null
     */
    public function getCompletedAt(): ?\DateTime
    {
        return $this->completedAt;
    }
    /**
     * @param \DateTime|null $completedAt
     *
     * @return self
     */
    public function setCompletedAt(?\DateTime $completedAt): self
    {
        $this->initialized['completedAt'] = true;
        $this->completedAt = $completedAt;
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
     * The start of the answer, only for an ingestion that keeps failed payloads
     *
     * @return string|null
     */
    public function getExcerpt(): ?string
    {
        return $this->excerpt;
    }
    /**
     * The start of the answer, only for an ingestion that keeps failed payloads
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
     * @return \DateTime|null
     */
    public function getExpiresAt(): ?\DateTime
    {
        return $this->expiresAt;
    }
    /**
     * @param \DateTime|null $expiresAt
     *
     * @return self
     */
    public function setExpiresAt(?\DateTime $expiresAt): self
    {
        $this->initialized['expiresAt'] = true;
        $this->expiresAt = $expiresAt;
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
    public function getIngestionId(): ?string
    {
        return $this->ingestionId;
    }
    /**
     * @param string|null $ingestionId
     *
     * @return self
     */
    public function setIngestionId(?string $ingestionId): self
    {
        $this->initialized['ingestionId'] = true;
        $this->ingestionId = $ingestionId;
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
     * Set while no live runtime carries the kind: the test waits, nothing has failed
     *
     * @return string|null
     */
    public function getNotice(): ?string
    {
        return $this->notice;
    }
    /**
     * Set while no live runtime carries the kind: the test waits, nothing has failed
     *
     * @param string|null $notice
     *
     * @return self
     */
    public function setNotice(?string $notice): self
    {
        $this->initialized['notice'] = true;
        $this->notice = $notice;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getOutcome(): ?string
    {
        return $this->outcome;
    }
    /**
     * @param string|null $outcome
     *
     * @return self
     */
    public function setOutcome(?string $outcome): self
    {
        $this->initialized['outcome'] = true;
        $this->outcome = $outcome;
        return $this;
    }
    /**
     * How many records the split makes of the answer, when it answered
     *
     * @return int|null
     */
    public function getRecords(): ?int
    {
        return $this->records;
    }
    /**
     * How many records the split makes of the answer, when it answered
     *
     * @param int|null $records
     *
     * @return self
     */
    public function setRecords(?int $records): self
    {
        $this->initialized['records'] = true;
        $this->records = $records;
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
     * PENDING until a runtime takes it, RUNNING while it tries, DONE with its result, EXPIRED when none answered in time
     *
     * @return string|null
     */
    public function getStatus(): ?string
    {
        return $this->status;
    }
    /**
     * PENDING until a runtime takes it, RUNNING while it tries, DONE with its result, EXPIRED when none answered in time
     *
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
     * What the runtime did, step by step, for a person
     *
     * @return list<string>|null
     */
    public function getSteps(): ?array
    {
        return $this->steps;
    }
    /**
     * What the runtime did, step by step, for a person
     *
     * @param list<string>|null $steps
     *
     * @return self
     */
    public function setSteps(?array $steps): self
    {
        $this->initialized['steps'] = true;
        $this->steps = $steps;
        return $this;
    }
}