<?php

namespace FlowCatalyst\Generated\Model;

class CreateJobsResponse
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
     * Jobs or events made by this request; records answered with an existing one are not counted
     *
     * @var int|null
     */
    protected $created;
    /**
     * What the ingestion's target makes of a record; results[].id is a dispatch job's id or an event's
     *
     * @var string|null
     */
    protected $kind;
    /**
     * @var list<JobResult>|null
     */
    protected $results;
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
     * Jobs or events made by this request; records answered with an existing one are not counted
     *
     * @return int|null
     */
    public function getCreated(): ?int
    {
        return $this->created;
    }
    /**
     * Jobs or events made by this request; records answered with an existing one are not counted
     *
     * @param int|null $created
     *
     * @return self
     */
    public function setCreated(?int $created): self
    {
        $this->initialized['created'] = true;
        $this->created = $created;
        return $this;
    }
    /**
     * What the ingestion's target makes of a record; results[].id is a dispatch job's id or an event's
     *
     * @return string|null
     */
    public function getKind(): ?string
    {
        return $this->kind;
    }
    /**
     * What the ingestion's target makes of a record; results[].id is a dispatch job's id or an event's
     *
     * @param string|null $kind
     *
     * @return self
     */
    public function setKind(?string $kind): self
    {
        $this->initialized['kind'] = true;
        $this->kind = $kind;
        return $this;
    }
    /**
     * @return list<JobResult>|null
     */
    public function getResults(): ?array
    {
        return $this->results;
    }
    /**
     * @param list<JobResult>|null $results
     *
     * @return self
     */
    public function setResults(?array $results): self
    {
        $this->initialized['results'] = true;
        $this->results = $results;
        return $this;
    }
}