<?php

namespace FlowCatalyst\Generated\Model;

class TraceLogResponse
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
     * A URL to the JSON Schema for this object.
     *
     * @var string|null
     */
    protected $dollarSchema;
    /**
     * Pass as cursor to fetch the next (older) page; null on the last page
     *
     * @var string|null
     */
    protected $nextCursor;
    /**
     * @var list<TraceRecordResponse>|null
     */
    protected $records;
    /**
     * A URL to the JSON Schema for this object.
     *
     * @return string|null
     */
    public function getDollarSchema(): ?string
    {
        return $this->dollarSchema;
    }
    /**
     * A URL to the JSON Schema for this object.
     *
     * @param string|null $dollarSchema
     *
     * @return self
     */
    public function setDollarSchema(?string $dollarSchema): self
    {
        $this->initialized['dollarSchema'] = true;
        $this->dollarSchema = $dollarSchema;
        return $this;
    }
    /**
     * Pass as cursor to fetch the next (older) page; null on the last page
     *
     * @return string|null
     */
    public function getNextCursor(): ?string
    {
        return $this->nextCursor;
    }
    /**
     * Pass as cursor to fetch the next (older) page; null on the last page
     *
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
    /**
     * @return list<TraceRecordResponse>|null
     */
    public function getRecords(): ?array
    {
        return $this->records;
    }
    /**
     * @param list<TraceRecordResponse>|null $records
     *
     * @return self
     */
    public function setRecords(?array $records): self
    {
        $this->initialized['records'] = true;
        $this->records = $records;
        return $this;
    }
}