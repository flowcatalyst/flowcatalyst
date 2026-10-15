<?php

namespace FlowCatalyst\Generated\Model;

class PutAliasResponse
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
     * @var AliasResponse|null
     */
    protected $alias;
    /**
     * @var WiringResponse|null
     */
    protected $wiring;
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
     * @return AliasResponse|null
     */
    public function getAlias(): ?AliasResponse
    {
        return $this->alias;
    }
    /**
     * @param AliasResponse|null $alias
     *
     * @return self
     */
    public function setAlias(?AliasResponse $alias): self
    {
        $this->initialized['alias'] = true;
        $this->alias = $alias;
        return $this;
    }
    /**
     * @return WiringResponse|null
     */
    public function getWiring(): ?WiringResponse
    {
        return $this->wiring;
    }
    /**
     * @param WiringResponse|null $wiring
     *
     * @return self
     */
    public function setWiring(?WiringResponse $wiring): self
    {
        $this->initialized['wiring'] = true;
        $this->wiring = $wiring;
        return $this;
    }
}