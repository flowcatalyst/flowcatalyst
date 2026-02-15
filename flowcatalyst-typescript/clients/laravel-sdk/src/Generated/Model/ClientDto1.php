<?php

namespace FlowCatalyst\Generated\Model;

class ClientDto1 extends \ArrayObject
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
     * Client ID
     *
     * @var string|null
     */
    protected $id;
    /**
     * Client name
     *
     * @var string|null
     */
    protected $name;
    /**
     * Client identifier/slug
     *
     * @var string|null
     */
    protected $identifier;
    /**
     * Client status (ACTIVE, SUSPENDED, INACTIVE)
     *
     * @var string|null
     */
    protected $status;
    /**
     * Reason for current status
     *
     * @var string|null
     */
    protected $statusReason;
    /**
     * @var \DateTime|null
     */
    protected $statusChangedAt;
    /**
     * @var \DateTime|null
     */
    protected $createdAt;
    /**
     * @var \DateTime|null
     */
    protected $updatedAt;
    /**
     * Client ID
     *
     * @return string|null
     */
    public function getId(): ?string
    {
        return $this->id;
    }
    /**
     * Client ID
     *
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
     * Client name
     *
     * @return string|null
     */
    public function getName(): ?string
    {
        return $this->name;
    }
    /**
     * Client name
     *
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
     * Client identifier/slug
     *
     * @return string|null
     */
    public function getIdentifier(): ?string
    {
        return $this->identifier;
    }
    /**
     * Client identifier/slug
     *
     * @param string|null $identifier
     *
     * @return self
     */
    public function setIdentifier(?string $identifier): self
    {
        $this->initialized['identifier'] = true;
        $this->identifier = $identifier;
        return $this;
    }
    /**
     * Client status (ACTIVE, SUSPENDED, INACTIVE)
     *
     * @return string|null
     */
    public function getStatus(): ?string
    {
        return $this->status;
    }
    /**
     * Client status (ACTIVE, SUSPENDED, INACTIVE)
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
     * Reason for current status
     *
     * @return string|null
     */
    public function getStatusReason(): ?string
    {
        return $this->statusReason;
    }
    /**
     * Reason for current status
     *
     * @param string|null $statusReason
     *
     * @return self
     */
    public function setStatusReason(?string $statusReason): self
    {
        $this->initialized['statusReason'] = true;
        $this->statusReason = $statusReason;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getStatusChangedAt(): ?\DateTime
    {
        return $this->statusChangedAt;
    }
    /**
     * @param \DateTime|null $statusChangedAt
     *
     * @return self
     */
    public function setStatusChangedAt(?\DateTime $statusChangedAt): self
    {
        $this->initialized['statusChangedAt'] = true;
        $this->statusChangedAt = $statusChangedAt;
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