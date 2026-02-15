<?php

namespace FlowCatalyst\Generated\Model;

class AuditLogListResponse extends \ArrayObject
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
     * Total count of matching entries
     *
     * @var int|null
     */
    protected $total;
    /**
     * Current page (0-based)
     *
     * @var int|null
     */
    protected $page;
    /**
     * Page size
     *
     * @var int|null
     */
    protected $pageSize;
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
     * Total count of matching entries
     *
     * @return int|null
     */
    public function getTotal(): ?int
    {
        return $this->total;
    }
    /**
     * Total count of matching entries
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
     * Current page (0-based)
     *
     * @return int|null
     */
    public function getPage(): ?int
    {
        return $this->page;
    }
    /**
     * Current page (0-based)
     *
     * @param int|null $page
     *
     * @return self
     */
    public function setPage(?int $page): self
    {
        $this->initialized['page'] = true;
        $this->page = $page;
        return $this;
    }
    /**
     * Page size
     *
     * @return int|null
     */
    public function getPageSize(): ?int
    {
        return $this->pageSize;
    }
    /**
     * Page size
     *
     * @param int|null $pageSize
     *
     * @return self
     */
    public function setPageSize(?int $pageSize): self
    {
        $this->initialized['pageSize'] = true;
        $this->pageSize = $pageSize;
        return $this;
    }
}