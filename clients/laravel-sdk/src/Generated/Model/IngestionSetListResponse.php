<?php

namespace FlowCatalyst\Generated\Model;

class IngestionSetListResponse
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
     * @var list<IngestionSetResponse>|null
     */
    protected $sets;
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
     * @return list<IngestionSetResponse>|null
     */
    public function getSets(): ?array
    {
        return $this->sets;
    }
    /**
     * @param list<IngestionSetResponse>|null $sets
     *
     * @return self
     */
    public function setSets(?array $sets): self
    {
        $this->initialized['sets'] = true;
        $this->sets = $sets;
        return $this;
    }
}