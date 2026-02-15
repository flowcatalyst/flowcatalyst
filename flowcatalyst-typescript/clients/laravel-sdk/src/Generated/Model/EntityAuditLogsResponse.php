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
     * Audit log entries
     *
     * @var list<AuditLogDto>|null
     */
    protected $auditLogs;
    /**
     * Total count
     *
     * @var int|null
     */
    protected $total;
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
     * Audit log entries
     *
     * @return list<AuditLogDto>|null
     */
    public function getAuditLogs(): ?array
    {
        return $this->auditLogs;
    }
    /**
     * Audit log entries
     *
     * @param list<AuditLogDto>|null $auditLogs
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
     * Total count
     *
     * @return int|null
     */
    public function getTotal(): ?int
    {
        return $this->total;
    }
    /**
     * Total count
     *
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
}