<?php

namespace FlowCatalyst\Generated\Model;

class Failure extends \ArrayObject
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
     * How many of the payload's records were read before it failed
     *
     * @var int|null
     */
    protected $afterRecords;
    /**
     * @var \DateTime|null
     */
    protected $at;
    /**
     * payload (the default): a record or payload the runtime could not take. rejected: the platform refused the runtime's jobs request permanently; the ingestion shows it as a notice until someone dismisses it.
     *
     * @var string|null
     */
    protected $category;
    /**
     * The start of what it failed on; the platform keeps it only for an ingestion that keeps failed payloads
     *
     * @var string|null
     */
    protected $excerpt;
    /**
     * @var string|null
     */
    protected $ingestionId;
    /**
     * @var string|null
     */
    protected $kind;
    /**
     * @var string|null
     */
    protected $message;
    /**
     * @var int|null
     */
    protected $version;
    /**
     * How many of the payload's records were read before it failed
     *
     * @return int|null
     */
    public function getAfterRecords(): ?int
    {
        return $this->afterRecords;
    }
    /**
     * How many of the payload's records were read before it failed
     *
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
     * @return \DateTime|null
     */
    public function getAt(): ?\DateTime
    {
        return $this->at;
    }
    /**
     * @param \DateTime|null $at
     *
     * @return self
     */
    public function setAt(?\DateTime $at): self
    {
        $this->initialized['at'] = true;
        $this->at = $at;
        return $this;
    }
    /**
     * payload (the default): a record or payload the runtime could not take. rejected: the platform refused the runtime's jobs request permanently; the ingestion shows it as a notice until someone dismisses it.
     *
     * @return string|null
     */
    public function getCategory(): ?string
    {
        return $this->category;
    }
    /**
     * payload (the default): a record or payload the runtime could not take. rejected: the platform refused the runtime's jobs request permanently; the ingestion shows it as a notice until someone dismisses it.
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
     * The start of what it failed on; the platform keeps it only for an ingestion that keeps failed payloads
     *
     * @return string|null
     */
    public function getExcerpt(): ?string
    {
        return $this->excerpt;
    }
    /**
     * The start of what it failed on; the platform keeps it only for an ingestion that keeps failed payloads
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