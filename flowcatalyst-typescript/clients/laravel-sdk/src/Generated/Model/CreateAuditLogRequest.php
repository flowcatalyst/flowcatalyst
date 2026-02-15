<?php

namespace FlowCatalyst\Generated\Model;

class CreateAuditLogRequest extends \ArrayObject
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
    protected $operationData;
    /**
     * @var string|null
     */
    protected $principalId;
    /**
     * @var \DateTime|null
     */
    protected $performedAt;
    /**
     * @var string|null
     */
    protected $source;
    /**
     * @var string|null
     */
    protected $correlationId;
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
    public function getOperationData(): ?string
    {
        return $this->operationData;
    }
    /**
     * @param string|null $operationData
     *
     * @return self
     */
    public function setOperationData(?string $operationData): self
    {
        $this->initialized['operationData'] = true;
        $this->operationData = $operationData;
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
    /**
     * @return string|null
     */
    public function getSource(): ?string
    {
        return $this->source;
    }
    /**
     * @param string|null $source
     *
     * @return self
     */
    public function setSource(?string $source): self
    {
        $this->initialized['source'] = true;
        $this->source = $source;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getCorrelationId(): ?string
    {
        return $this->correlationId;
    }
    /**
     * @param string|null $correlationId
     *
     * @return self
     */
    public function setCorrelationId(?string $correlationId): self
    {
        $this->initialized['correlationId'] = true;
        $this->correlationId = $correlationId;
        return $this;
    }
}