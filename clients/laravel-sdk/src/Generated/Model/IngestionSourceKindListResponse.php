<?php

namespace FlowCatalyst\Generated\Model;

class IngestionSourceKindListResponse
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
     * @var list<IngestionSourceKindResponse>|null
     */
    protected $kinds;
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
     * @return list<IngestionSourceKindResponse>|null
     */
    public function getKinds(): ?array
    {
        return $this->kinds;
    }
    /**
     * @param list<IngestionSourceKindResponse>|null $kinds
     *
     * @return self
     */
    public function setKinds(?array $kinds): self
    {
        $this->initialized['kinds'] = true;
        $this->kinds = $kinds;
        return $this;
    }
}