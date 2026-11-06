<?php

namespace FlowCatalyst\Generated\Model;

class BulkIssueFieldBadgesRequest extends \ArrayObject
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
     * 1 to 200 workers, all in clients the caller can access; otherwise nothing is issued
     *
     * @var list<string>|null
     */
    protected $workerIds;
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
     * 1 to 200 workers, all in clients the caller can access; otherwise nothing is issued
     *
     * @return list<string>|null
     */
    public function getWorkerIds(): ?array
    {
        return $this->workerIds;
    }
    /**
     * 1 to 200 workers, all in clients the caller can access; otherwise nothing is issued
     *
     * @param list<string>|null $workerIds
     *
     * @return self
     */
    public function setWorkerIds(?array $workerIds): self
    {
        $this->initialized['workerIds'] = true;
        $this->workerIds = $workerIds;
        return $this;
    }
}