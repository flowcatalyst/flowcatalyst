<?php

namespace FlowCatalyst\Generated\Model;

class TraceContextResponse
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
     * True for the built-in http context
     *
     * @var bool|null
     */
    protected $builtIn;
    /**
     * @var string|null
     */
    protected $description;
    /**
     * @var \DateTime|null
     */
    protected $enabledUntil;
    /**
     * Switched on with no time limit; it stays on until switched off (enabledUntil is then far in the future)
     *
     * @var bool|null
     */
    protected $indefinite;
    /**
     * The window is open now (enabledUntil is in the future)
     *
     * @var bool|null
     */
    protected $live;
    /**
     * The longest window this context may be enabled for
     *
     * @var int|null
     */
    protected $maxMinutes;
    /**
     * @var string|null
     */
    protected $name;
    /**
     * One invocation in N is captured; 1 = all
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
     * True for the built-in http context
     *
     * @return bool|null
     */
    public function getBuiltIn(): ?bool
    {
        return $this->builtIn;
    }
    /**
     * True for the built-in http context
     *
     * @param bool|null $builtIn
     *
     * @return self
     */
    public function setBuiltIn(?bool $builtIn): self
    {
        $this->initialized['builtIn'] = true;
        $this->builtIn = $builtIn;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getDescription(): ?string
    {
        return $this->description;
    }
    /**
     * @param string|null $description
     *
     * @return self
     */
    public function setDescription(?string $description): self
    {
        $this->initialized['description'] = true;
        $this->description = $description;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getEnabledUntil(): ?\DateTime
    {
        return $this->enabledUntil;
    }
    /**
     * @param \DateTime|null $enabledUntil
     *
     * @return self
     */
    public function setEnabledUntil(?\DateTime $enabledUntil): self
    {
        $this->initialized['enabledUntil'] = true;
        $this->enabledUntil = $enabledUntil;
        return $this;
    }
    /**
     * Switched on with no time limit; it stays on until switched off (enabledUntil is then far in the future)
     *
     * @return bool|null
     */
    public function getIndefinite(): ?bool
    {
        return $this->indefinite;
    }
    /**
     * Switched on with no time limit; it stays on until switched off (enabledUntil is then far in the future)
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
     * The window is open now (enabledUntil is in the future)
     *
     * @return bool|null
     */
    public function getLive(): ?bool
    {
        return $this->live;
    }
    /**
     * The window is open now (enabledUntil is in the future)
     *
     * @param bool|null $live
     *
     * @return self
     */
    public function setLive(?bool $live): self
    {
        $this->initialized['live'] = true;
        $this->live = $live;
        return $this;
    }
    /**
     * The longest window this context may be enabled for
     *
     * @return int|null
     */
    public function getMaxMinutes(): ?int
    {
        return $this->maxMinutes;
    }
    /**
     * The longest window this context may be enabled for
     *
     * @param int|null $maxMinutes
     *
     * @return self
     */
    public function setMaxMinutes(?int $maxMinutes): self
    {
        $this->initialized['maxMinutes'] = true;
        $this->maxMinutes = $maxMinutes;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getName(): ?string
    {
        return $this->name;
    }
    /**
     * @param string|null $name
     *
     * @return self
     */
    public function setName(?string $name): self
    {
        $this->initialized['name'] = true;
        $this->name = $name;
        return $this;
    }
    /**
     * One invocation in N is captured; 1 = all
     *
     * @return int|null
     */
    public function getSampleRate(): ?int
    {
        return $this->sampleRate;
    }
    /**
     * One invocation in N is captured; 1 = all
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