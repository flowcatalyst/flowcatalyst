<?php

namespace FlowCatalyst\Generated\Model;

class TraceState
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
    protected $context;
    /**
     * @var int|null
     */
    protected $enabledUntilMs;
    /**
     * @var int|null
     */
    protected $sampleRate;
    /**
     * @return string|null
     */
    public function getContext(): ?string
    {
        return $this->context;
    }
    /**
     * @param string|null $context
     *
     * @return self
     */
    public function setContext(?string $context): self
    {
        $this->initialized['context'] = true;
        $this->context = $context;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getEnabledUntilMs(): ?int
    {
        return $this->enabledUntilMs;
    }
    /**
     * @param int|null $enabledUntilMs
     *
     * @return self
     */
    public function setEnabledUntilMs(?int $enabledUntilMs): self
    {
        $this->initialized['enabledUntilMs'] = true;
        $this->enabledUntilMs = $enabledUntilMs;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getSampleRate(): ?int
    {
        return $this->sampleRate;
    }
    /**
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