<?php

namespace FlowCatalyst\Generated\Model;

class EntityAuditLogsResponse extends \ArrayObject
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
     * @var list<AuditLogResponse>|null
     */
    protected $auditLogs;
    /**
     * @var string|null
     */
    protected $entityId;
    /**
     * @var string|null
     */
    protected $entityType;
    /**
     * @var int|null
     */
    protected $total;
    /**
     * @return list<AuditLogResponse>|null
     */
    public function getAuditLogs(): ?array
    {
        return $this->auditLogs;
    }
    /**
     * @param list<AuditLogResponse>|null $auditLogs
     *
     * @return self
     */
    public function setAuditLogs(?array $auditLogs): self
    {
        $this->initialized['auditLogs'] = true;
        $this->auditLogs = $auditLogs;
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
     * @return int|null
     */
    public function getTotal(): ?int
    {
        return $this->total;
    }
    /**
     * @param int|null $total
     *
     * @return self
     */
    public function setTotal(?int $total): self
    {
        $this->initialized['total'] = true;
        $this->total = $total;
        return $this;
    }
}