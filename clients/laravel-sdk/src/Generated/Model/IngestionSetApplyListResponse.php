<?php

namespace FlowCatalyst\Generated\Model;

class IngestionSetApplyListResponse
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
     * @var list<IngestionSetApplyResponse>|null
     */
    protected $applies;
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
     * @return list<IngestionSetApplyResponse>|null
     */
    public function getApplies(): ?array
    {
        return $this->applies;
    }
    /**
     * @param list<IngestionSetApplyResponse>|null $applies
     *
     * @return self
     */
    public function setApplies(?array $applies): self
    {
        $this->initialized['applies'] = true;
        $this->applies = $applies;
        return $this;
    }
}