<?php

namespace FlowCatalyst\Generated\Model;

use FlowCatalyst\Generated\Runtime\AdditionalAndPatternProperties;
use FlowCatalyst\Generated\Runtime\AdditionalPropertiesInterface;
class IngestionTraceBatch implements AdditionalPropertiesInterface
{
    use AdditionalAndPatternProperties;
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
     * @var list<IngestionTraceRecord>|null
     */
    protected $records;
    /**
     * @var string|null
     */
    protected $runtimeId;
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
     * @return list<IngestionTraceRecord>|null
     */
    public function getRecords(): ?array
    {
        return $this->records;
    }
    /**
     * @param list<IngestionTraceRecord>|null $records
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
    public function getRuntimeId(): ?string
    {
        return $this->runtimeId;
    }
    /**
     * @param string|null $runtimeId
     *
     * @return self
     */
    public function setRuntimeId(?string $runtimeId): self
    {
        $this->initialized['runtimeId'] = true;
        $this->runtimeId = $runtimeId;
        return $this;
    }
    public function definedProperties(): array
    {
        return ['dollarSchema' => ['$schema', 'getDollarSchema', 'setDollarSchema'], 'records' => ['records', 'getRecords', 'setRecords'], 'runtimeId' => ['runtimeId', 'getRuntimeId', 'setRuntimeId']];
    }
}