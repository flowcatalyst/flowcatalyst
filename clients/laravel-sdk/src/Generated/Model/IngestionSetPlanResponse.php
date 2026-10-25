<?php

namespace FlowCatalyst\Generated\Model;

class IngestionSetPlanResponse
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
     * The recorded apply; absent for a dry run and for an apply that changed nothing
     *
     * @var string|null
     */
    protected $applyId;
    /**
     * Each item of the set, then each ingestion removed from it
     *
     * @var list<SetChange>|null
     */
    protected $changes;
    /**
     * @var bool|null
     */
    protected $dryRun;
    /**
     * @var string|null
     */
    protected $name;
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
     * The recorded apply; absent for a dry run and for an apply that changed nothing
     *
     * @return string|null
     */
    public function getApplyId(): ?string
    {
        return $this->applyId;
    }
    /**
     * The recorded apply; absent for a dry run and for an apply that changed nothing
     *
     * @param string|null $applyId
     *
     * @return self
     */
    public function setApplyId(?string $applyId): self
    {
        $this->initialized['applyId'] = true;
        $this->applyId = $applyId;
        return $this;
    }
    /**
     * Each item of the set, then each ingestion removed from it
     *
     * @return list<SetChange>|null
     */
    public function getChanges(): ?array
    {
        return $this->changes;
    }
    /**
     * Each item of the set, then each ingestion removed from it
     *
     * @param list<SetChange>|null $changes
     *
     * @return self
     */
    public function setChanges(?array $changes): self
    {
        $this->initialized['changes'] = true;
        $this->changes = $changes;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getDryRun(): ?bool
    {
        return $this->dryRun;
    }
    /**
     * @param bool|null $dryRun
     *
     * @return self
     */
    public function setDryRun(?bool $dryRun): self
    {
        $this->initialized['dryRun'] = true;
        $this->dryRun = $dryRun;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getName(): ?string
    {
        return $this->name;
    }
    /**
     * @param string|null $name
     *
     * @return self
     */
    public function setName(?string $name): self
    {
        $this->initialized['name'] = true;
        $this->name = $name;
        return $this;
    }
}