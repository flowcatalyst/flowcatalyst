<?php

namespace FlowCatalyst\Generated\Model;

class DeleteClientPhotosResponse
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
     * How many photos were deleted; 0 when the client held none
     *
     * @var int|null
     */
    protected $removed;
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
     * How many photos were deleted; 0 when the client held none
     *
     * @return int|null
     */
    public function getRemoved(): ?int
    {
        return $this->removed;
    }
    /**
     * How many photos were deleted; 0 when the client held none
     *
     * @param int|null $removed
     *
     * @return self
     */
    public function setRemoved(?int $removed): self
    {
        $this->initialized['removed'] = true;
        $this->removed = $removed;
        return $this;
    }
}