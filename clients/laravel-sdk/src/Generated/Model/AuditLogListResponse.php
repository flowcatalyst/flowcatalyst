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
     * @var list<AuditLogResponse>|null
     */
    protected $auditLogs;
    /**
     * @var bool|null
     */
    protected $hasMore;
    /**
     * @var string|null
     */
    protected $nextCursor;
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
     * @return bool|null
     */
    public function getHasMore(): ?bool
    {
        return $this->hasMore;
    }
    /**
     * @param bool|null $hasMore
     *
     * @return self
     */
    public function setHasMore(?bool $hasMore): self
    {
        $this->initialized['hasMore'] = true;
        $this->hasMore = $hasMore;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getNextCursor(): ?string
    {
        return $this->nextCursor;
    }
    /**
     * @param string|null $nextCursor
     *
     * @return self
     */
    public function setNextCursor(?string $nextCursor): self
    {
        $this->initialized['nextCursor'] = true;
        $this->nextCursor = $nextCursor;
        return $this;
    }
}