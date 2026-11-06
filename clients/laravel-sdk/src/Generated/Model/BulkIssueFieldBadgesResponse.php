<?php

namespace FlowCatalyst\Generated\Model;

class BulkIssueFieldBadgesResponse
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
     * @var list<BulkIssuedFieldBadge>|null
     */
    protected $issued;
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
     * @return list<BulkIssuedFieldBadge>|null
     */
    public function getIssued(): ?array
    {
        return $this->issued;
    }
    /**
     * @param list<BulkIssuedFieldBadge>|null $issued
     *
     * @return self
     */
    public function setIssued(?array $issued): self
    {
        $this->initialized['issued'] = true;
        $this->issued = $issued;
        return $this;
    }
}