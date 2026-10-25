<?php

namespace FlowCatalyst\Generated\Model;

class EnableTraceRequest extends \ArrayObject
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
     * Stay on until switched off, at most the context's indefiniteMaxDays (then it closes by itself); minutes is ignored. Needs platform:function:trace:indefinite; never for http (400 TRACE_INDEFINITE_NOT_ALLOWED)
     *
     * @var bool|null
     */
    protected $indefinite;
    /**
     * How long the window stays open, from 1 up to the context's maxMinutes. Required unless indefinite.
     *
     * @var int|null
     */
    protected $minutes;
    /**
     * Capture one invocation in N (1 to 1000). Default 1: all.
     *
     * @var int|null
     */
    protected $sampleRate;
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
     * Stay on until switched off, at most the context's indefiniteMaxDays (then it closes by itself); minutes is ignored. Needs platform:function:trace:indefinite; never for http (400 TRACE_INDEFINITE_NOT_ALLOWED)
     *
     * @return bool|null
     */
    public function getIndefinite(): ?bool
    {
        return $this->indefinite;
    }
    /**
     * Stay on until switched off, at most the context's indefiniteMaxDays (then it closes by itself); minutes is ignored. Needs platform:function:trace:indefinite; never for http (400 TRACE_INDEFINITE_NOT_ALLOWED)
     *
     * @param bool|null $indefinite
     *
     * @return self
     */
    public function setIndefinite(?bool $indefinite): self
    {
        $this->initialized['indefinite'] = true;
        $this->indefinite = $indefinite;
        return $this;
    }
    /**
     * How long the window stays open, from 1 up to the context's maxMinutes. Required unless indefinite.
     *
     * @return int|null
     */
    public function getMinutes(): ?int
    {
        return $this->minutes;
    }
    /**
     * How long the window stays open, from 1 up to the context's maxMinutes. Required unless indefinite.
     *
     * @param int|null $minutes
     *
     * @return self
     */
    public function setMinutes(?int $minutes): self
    {
        $this->initialized['minutes'] = true;
        $this->minutes = $minutes;
        return $this;
    }
    /**
     * Capture one invocation in N (1 to 1000). Default 1: all.
     *
     * @return int|null
     */
    public function getSampleRate(): ?int
    {
        return $this->sampleRate;
    }
    /**
     * Capture one invocation in N (1 to 1000). Default 1: all.
     *
     * @param int|null $sampleRate
     *
     * @return self
     */
    public function setSampleRate(?int $sampleRate): self
    {
        $this->initialized['sampleRate'] = true;
        $this->sampleRate = $sampleRate;
        return $this;
    }
}