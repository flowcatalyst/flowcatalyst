<?php

namespace FlowCatalyst\Generated\Model;

class PruneVersionsRequest extends \ArrayObject
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
     * Report what would be pruned and change nothing
     *
     * @var bool|null
     */
    protected $dryRun;
    /**
     * Untagged versions to keep (>= 1); omitted uses the platform setting FC_FUNCTIONS_KEEP_VERSIONS (default 20)
     *
     * @var int|null
     */
    protected $keep;
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
     * Report what would be pruned and change nothing
     *
     * @return bool|null
     */
    public function getDryRun(): ?bool
    {
        return $this->dryRun;
    }
    /**
     * Report what would be pruned and change nothing
     *
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
     * Untagged versions to keep (>= 1); omitted uses the platform setting FC_FUNCTIONS_KEEP_VERSIONS (default 20)
     *
     * @return int|null
     */
    public function getKeep(): ?int
    {
        return $this->keep;
    }
    /**
     * Untagged versions to keep (>= 1); omitted uses the platform setting FC_FUNCTIONS_KEEP_VERSIONS (default 20)
     *
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
}