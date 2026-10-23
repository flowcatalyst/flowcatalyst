<?php

namespace FlowCatalyst\Generated\Model;

class TraceBatch extends \ArrayObject
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
     * @var string|null
     */
    protected $pool;
    /**
     * @var list<TraceRecord>|null
     */
    protected $records;
    /**
     * @var string|null
     */
    protected $runnerId;
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
     * @return string|null
     */
    public function getPool(): ?string
    {
        return $this->pool;
    }
    /**
     * @param string|null $pool
     *
     * @return self
     */
    public function setPool(?string $pool): self
    {
        $this->initialized['pool'] = true;
        $this->pool = $pool;
        return $this;
    }
    /**
     * @return list<TraceRecord>|null
     */
    public function getRecords(): ?array
    {
        return $this->records;
    }
    /**
     * @param list<TraceRecord>|null $records
     *
     * @return self
     */
    public function setRecords(?array $records): self
    {
        $this->initialized['records'] = true;
        $this->records = $records;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getRunnerId(): ?string
    {
        return $this->runnerId;
    }
    /**
     * @param string|null $runnerId
     *
     * @return self
     */
    public function setRunnerId(?string $runnerId): self
    {
        $this->initialized['runnerId'] = true;
        $this->runnerId = $runnerId;
        return $this;
    }
}