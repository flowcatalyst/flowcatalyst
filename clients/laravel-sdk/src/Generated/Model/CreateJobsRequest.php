<?php

namespace FlowCatalyst\Generated\Model;

class CreateJobsRequest extends \ArrayObject
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
     * @var list<JobRecord>|null
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
     * @return list<JobRecord>|null
     */
    public function getRecords(): ?array
    {
        return $this->records;
    }
    /**
     * @param list<JobRecord>|null $records
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