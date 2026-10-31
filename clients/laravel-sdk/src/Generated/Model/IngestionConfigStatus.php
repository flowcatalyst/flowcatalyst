<?php

namespace FlowCatalyst\Generated\Model;

class IngestionConfigStatus
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
     * The entry's key; absent while the settings are inline
     *
     * @var string|null
     */
    protected $key;
    /**
     * Why the entry is INVALID
     *
     * @var list<FieldError>|null
     */
    protected $problems;
    /**
     * @var string|null
     */
    protected $status;
    /**
     * @var \DateTime|null
     */
    protected $updatedAt;
    /**
     * The entry's key; absent while the settings are inline
     *
     * @return string|null
     */
    public function getKey(): ?string
    {
        return $this->key;
    }
    /**
     * The entry's key; absent while the settings are inline
     *
     * @param string|null $key
     *
     * @return self
     */
    public function setKey(?string $key): self
    {
        $this->initialized['key'] = true;
        $this->key = $key;
        return $this;
    }
    /**
     * Why the entry is INVALID
     *
     * @return list<FieldError>|null
     */
    public function getProblems(): ?array
    {
        return $this->problems;
    }
    /**
     * Why the entry is INVALID
     *
     * @param list<FieldError>|null $problems
     *
     * @return self
     */
    public function setProblems(?array $problems): self
    {
        $this->initialized['problems'] = true;
        $this->problems = $problems;
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