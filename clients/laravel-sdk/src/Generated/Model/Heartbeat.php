<?php

namespace FlowCatalyst\Generated\Model;

class Heartbeat extends \ArrayObject
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
     * @var BudgetReport|null
     */
    protected $budget;
    /**
     * @var string|null
     */
    protected $pool;
    /**
     * @var int|null
     */
    protected $revision;
    /**
     * @var string|null
     */
    protected $runnerId;
    /**
     * @var \DateTime|null
     */
    protected $startedAt;
    /**
     * @var list<VersionReport>|null
     */
    protected $versions;
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
     * @return BudgetReport|null
     */
    public function getBudget(): ?BudgetReport
    {
        return $this->budget;
    }
    /**
     * @param BudgetReport|null $budget
     *
     * @return self
     */
    public function setBudget(?BudgetReport $budget): self
    {
        $this->initialized['budget'] = true;
        $this->budget = $budget;
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
     * @return int|null
     */
    public function getRevision(): ?int
    {
        return $this->revision;
    }
    /**
     * @param int|null $revision
     *
     * @return self
     */
    public function setRevision(?int $revision): self
    {
        $this->initialized['revision'] = true;
        $this->revision = $revision;
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
    /**
     * @return \DateTime|null
     */
    public function getStartedAt(): ?\DateTime
    {
        return $this->startedAt;
    }
    /**
     * @param \DateTime|null $startedAt
     *
     * @return self
     */
    public function setStartedAt(?\DateTime $startedAt): self
    {
        $this->initialized['startedAt'] = true;
        $this->startedAt = $startedAt;
        return $this;
    }
    /**
     * @return list<VersionReport>|null
     */
    public function getVersions(): ?array
    {
        return $this->versions;
    }
    /**
     * @param list<VersionReport>|null $versions
     *
     * @return self
     */
    public function setVersions(?array $versions): self
    {
        $this->initialized['versions'] = true;
        $this->versions = $versions;
        return $this;
    }
}