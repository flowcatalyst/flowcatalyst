<?php

namespace FlowCatalyst\Generated\Model;

class AuditLogDto extends \ArrayObject
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
    protected $id;
    /**
     * @var string|null
     */
    protected $entityType;
    /**
     * @var string|null
     */
    protected $entityId;
    /**
     * @var string|null
     */
    protected $operation;
    /**
     * @var string|null
     */
    protected $principalId;
    /**
     * @var string|null
     */
    protected $principalName;
    /**
     * @var \DateTime|null
     */
    protected $performedAt;
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
    public function getEntityType(): ?string
    {
        return $this->entityType;
    }
    /**
     * @param string|null $entityType
     *
     * @return self
     */
    public function setEntityType(?string $entityType): self
    {
        $this->initialized['entityType'] = true;
        $this->entityType = $entityType;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getEntityId(): ?string
    {
        return $this->entityId;
    }
    /**
     * @param string|null $entityId
     *
     * @return self
     */
    public function setEntityId(?string $entityId): self
    {
        $this->initialized['entityId'] = true;
        $this->entityId = $entityId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getOperation(): ?string
    {
        return $this->operation;
    }
    /**
     * @param string|null $operation
     *
     * @return self
     */
    public function setOperation(?string $operation): self
    {
        $this->initialized['operation'] = true;
        $this->operation = $operation;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getPrincipalId(): ?string
    {
        return $this->principalId;
    }
    /**
     * @param string|null $principalId
     *
     * @return self
     */
    public function setPrincipalId(?string $principalId): self
    {
        $this->initialized['principalId'] = true;
        $this->principalId = $principalId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getPrincipalName(): ?string
    {
        return $this->principalName;
    }
    /**
     * @param string|null $principalName
     *
     * @return self
     */
    public function setPrincipalName(?string $principalName): self
    {
        $this->initialized['principalName'] = true;
        $this->principalName = $principalName;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getPerformedAt(): ?\DateTime
    {
        return $this->performedAt;
    }
    /**
     * @param \DateTime|null $performedAt
     *
     * @return self
     */
    public function setPerformedAt(?\DateTime $performedAt): self
    {
        $this->initialized['performedAt'] = true;
        $this->performedAt = $performedAt;
        return $this;
    }
}