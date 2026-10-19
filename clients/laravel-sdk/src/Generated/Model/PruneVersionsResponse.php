<?php

namespace FlowCatalyst\Generated\Model;

class PruneVersionsResponse
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
     * Stored artifacts removed because no remaining version references them
     *
     * @var int|null
     */
    protected $artifactsDeleted;
    /**
     * @var bool|null
     */
    protected $dryRun;
    /**
     * @var int|null
     */
    protected $keep;
    /**
     * Versions deleted (would be deleted, on a dry run), oldest first
     *
     * @var list<PrunedVersionResponse>|null
     */
    protected $pruned;
    /**
     * Untagged versions before pruning
     *
     * @var int|null
     */
    protected $untagged;
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
     * Stored artifacts removed because no remaining version references them
     *
     * @return int|null
     */
    public function getArtifactsDeleted(): ?int
    {
        return $this->artifactsDeleted;
    }
    /**
     * Stored artifacts removed because no remaining version references them
     *
     * @param int|null $artifactsDeleted
     *
     * @return self
     */
    public function setArtifactsDeleted(?int $artifactsDeleted): self
    {
        $this->initialized['artifactsDeleted'] = true;
        $this->artifactsDeleted = $artifactsDeleted;
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
     * @return int|null
     */
    public function getKeep(): ?int
    {
        return $this->keep;
    }
    /**
     * @param int|null $keep
     *
     * @return self
     */
    public function setKeep(?int $keep): self
    {
        $this->initialized['keep'] = true;
        $this->keep = $keep;
        return $this;
    }
    /**
     * Versions deleted (would be deleted, on a dry run), oldest first
     *
     * @return list<PrunedVersionResponse>|null
     */
    public function getPruned(): ?array
    {
        return $this->pruned;
    }
    /**
     * Versions deleted (would be deleted, on a dry run), oldest first
     *
     * @param list<PrunedVersionResponse>|null $pruned
     *
     * @return self
     */
    public function setPruned(?array $pruned): self
    {
        $this->initialized['pruned'] = true;
        $this->pruned = $pruned;
        return $this;
    }
    /**
     * Untagged versions before pruning
     *
     * @return int|null
     */
    public function getUntagged(): ?int
    {
        return $this->untagged;
    }
    /**
     * Untagged versions before pruning
     *
     * @param int|null $untagged
     *
     * @return self
     */
    public function setUntagged(?int $untagged): self
    {
        $this->initialized['untagged'] = true;
        $this->untagged = $untagged;
        return $this;
    }
}