<?php

namespace FlowCatalyst\Generated\Model;

class VersionResponse
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
    protected $abi;
    /**
     * @var \DateTime|null
     */
    protected $createdAt;
    /**
     * @var mixed|null
     */
    protected $describe;
    /**
     * @var string|null
     */
    protected $digest;
    /**
     * @var mixed|null
     */
    protected $failure;
    /**
     * @var string|null
     */
    protected $functionId;
    /**
     * @var string|null
     */
    protected $id;
    /**
     * @var int|null
     */
    protected $number;
    /**
     * @var string|null
     */
    protected $publishedBy;
    /**
     * @var \DateTime|null
     */
    protected $readyAt;
    /**
     * @var string|null
     */
    protected $runtime;
    /**
     * @var int|null
     */
    protected $sizeBytes;
    /**
     * @var string|null
     */
    protected $status;
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
    public function getAbi(): ?int
    {
        return $this->abi;
    }
    /**
     * @param int|null $abi
     *
     * @return self
     */
    public function setAbi(?int $abi): self
    {
        $this->initialized['abi'] = true;
        $this->abi = $abi;
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
     * @return mixed
     */
    public function getDescribe()
    {
        return $this->describe;
    }
    /**
     * @param mixed $describe
     *
     * @return self
     */
    public function setDescribe($describe): self
    {
        $this->initialized['describe'] = true;
        $this->describe = $describe;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getDigest(): ?string
    {
        return $this->digest;
    }
    /**
     * @param string|null $digest
     *
     * @return self
     */
    public function setDigest(?string $digest): self
    {
        $this->initialized['digest'] = true;
        $this->digest = $digest;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getFailure()
    {
        return $this->failure;
    }
    /**
     * @param mixed $failure
     *
     * @return self
     */
    public function setFailure($failure): self
    {
        $this->initialized['failure'] = true;
        $this->failure = $failure;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getFunctionId(): ?string
    {
        return $this->functionId;
    }
    /**
     * @param string|null $functionId
     *
     * @return self
     */
    public function setFunctionId(?string $functionId): self
    {
        $this->initialized['functionId'] = true;
        $this->functionId = $functionId;
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
     * @return int|null
     */
    public function getNumber(): ?int
    {
        return $this->number;
    }
    /**
     * @param int|null $number
     *
     * @return self
     */
    public function setNumber(?int $number): self
    {
        $this->initialized['number'] = true;
        $this->number = $number;
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
     * @return \DateTime|null
     */
    public function getReadyAt(): ?\DateTime
    {
        return $this->readyAt;
    }
    /**
     * @param \DateTime|null $readyAt
     *
     * @return self
     */
    public function setReadyAt(?\DateTime $readyAt): self
    {
        $this->initialized['readyAt'] = true;
        $this->readyAt = $readyAt;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getRuntime(): ?string
    {
        return $this->runtime;
    }
    /**
     * @param string|null $runtime
     *
     * @return self
     */
    public function setRuntime(?string $runtime): self
    {
        $this->initialized['runtime'] = true;
        $this->runtime = $runtime;
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
}