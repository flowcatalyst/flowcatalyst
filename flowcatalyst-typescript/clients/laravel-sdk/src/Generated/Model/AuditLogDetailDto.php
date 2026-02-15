<?php

namespace FlowCatalyst\Generated\Model;

class AuditLogDetailDto extends \ArrayObject
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
     * Audit log ID
     *
     * @var string|null
     */
    protected $id;
    /**
     * Entity type
     *
     * @var string|null
     */
    protected $entityType;
    /**
     * Entity ID
     *
     * @var string|null
     */
    protected $entityId;
    /**
     * Operation performed
     *
     * @var string|null
     */
    protected $operation;
    /**
     * Full operation payload as JSON
     *
     * @var string|null
     */
    protected $operationJson;
    /**
     * Principal ID who performed the operation
     *
     * @var string|null
     */
    protected $principalId;
    /**
     * Principal name (resolved)
     *
     * @var string|null
     */
    protected $principalName;
    /**
     * @var \DateTime|null
     */
    protected $performedAt;
    /**
     * Audit log ID
     *
     * @return string|null
     */
    public function getId(): ?string
    {
        return $this->id;
    }
    /**
     * Audit log ID
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
     * Entity type
     *
     * @return string|null
     */
    public function getEntityType(): ?string
    {
        return $this->entityType;
    }
    /**
     * Entity type
     *
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
     * Entity ID
     *
     * @return string|null
     */
    public function getEntityId(): ?string
    {
        return $this->entityId;
    }
    /**
     * Entity ID
     *
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
     * Operation performed
     *
     * @return string|null
     */
    public function getOperation(): ?string
    {
        return $this->operation;
    }
    /**
     * Operation performed
     *
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
     * Full operation payload as JSON
     *
     * @return string|null
     */
    public function getOperationJson(): ?string
    {
        return $this->operationJson;
    }
    /**
     * Full operation payload as JSON
     *
     * @param string|null $operationJson
     *
     * @return self
     */
    public function setOperationJson(?string $operationJson): self
    {
        $this->initialized['operationJson'] = true;
        $this->operationJson = $operationJson;
        return $this;
    }
    /**
     * Principal ID who performed the operation
     *
     * @return string|null
     */
    public function getPrincipalId(): ?string
    {
        return $this->principalId;
    }
    /**
     * Principal ID who performed the operation
     *
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
     * Principal name (resolved)
     *
     * @return string|null
     */
    public function getPrincipalName(): ?string
    {
        return $this->principalName;
    }
    /**
     * Principal name (resolved)
     *
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