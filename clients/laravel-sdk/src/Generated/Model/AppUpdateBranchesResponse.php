<?php

namespace FlowCatalyst\Generated\Model;

class AppUpdateBranchesResponse
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
     * @var list<AppUpdateBranchResponse>|null
     */
    protected $branches;
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
     * @return list<AppUpdateBranchResponse>|null
     */
    public function getBranches(): ?array
    {
        return $this->branches;
    }
    /**
     * @param list<AppUpdateBranchResponse>|null $branches
     *
     * @return self
     */
    public function setBranches(?array $branches): self
    {
        $this->initialized['branches'] = true;
        $this->branches = $branches;
        return $this;
    }
}