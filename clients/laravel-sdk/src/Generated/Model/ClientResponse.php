<?php

namespace FlowCatalyst\Generated\Model;

class ClientResponse extends \ArrayObject
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
     * @var string|null
     */
    protected $createdAt;
    /**
     * @var string|null
     */
    protected $id;
    /**
     * @var string|null
     */
    protected $identifier;
    /**
     * @var string|null
     */
    protected $name;
    /**
     * @var string|null
     */
    protected $status;
    /**
     * @var string|null
     */
    protected $statusChangedAt;
    /**
     * @var string|null
     */
    protected $statusReason;
    /**
     * @var string|null
     */
    protected $updatedAt;
    /**
     * @return string|null
     */
    public function getCreatedAt(): ?string
    {
        return $this->createdAt;
    }
    /**
     * @param string|null $createdAt
     *
     * @return self
     */
    public function setCreatedAt(?string $createdAt): self
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
    public function getIdentifier(): ?string
    {
        return $this->identifier;
    }
    /**
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
     * @return string|null
     */
    public function getStatusChangedAt(): ?string
    {
        return $this->statusChangedAt;
    }
    /**
     * @param string|null $statusChangedAt
     *
     * @return self
     */
    public function setStatusChangedAt(?string $statusChangedAt): self
    {
        $this->initialized['statusChangedAt'] = true;
        $this->statusChangedAt = $statusChangedAt;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getStatusReason(): ?string
    {
        return $this->statusReason;
    }
    /**
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
     * @return string|null
     */
    public function getUpdatedAt(): ?string
    {
        return $this->updatedAt;
    }
    /**
     * @param string|null $updatedAt
     *
     * @return self
     */
    public function setUpdatedAt(?string $updatedAt): self
    {
        $this->initialized['updatedAt'] = true;
        $this->updatedAt = $updatedAt;
        return $this;
    }
}