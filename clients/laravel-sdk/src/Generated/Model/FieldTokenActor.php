<?php

namespace FlowCatalyst\Generated\Model;

class FieldTokenActor
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
     * @var string|null
     */
    protected $sub;
    /**
     * @var string|null
     */
    protected $typ;
    /**
     * @return string|null
     */
    public function getSub(): ?string
    {
        return $this->sub;
    }
    /**
     * @param string|null $sub
     *
     * @return self
     */
    public function setSub(?string $sub): self
    {
        $this->initialized['sub'] = true;
        $this->sub = $sub;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getTyp(): ?string
    {
        return $this->typ;
    }
    /**
     * @param string|null $typ
     *
     * @return self
     */
    public function setTyp(?string $typ): self
    {
        $this->initialized['typ'] = true;
        $this->typ = $typ;
        return $this;
    }
}