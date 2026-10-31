<?php

namespace FlowCatalyst\Generated\Model;

class RuntimeTrace
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
     * The runtime never captures past this, and the platform refuses records dated after it (plus a short grace)
     *
     * @var \DateTime|null
     */
    protected $hardStop;
    /**
     * @var string|null
     */
    protected $ingestionId;
    /**
     * The runtime starts a capture of this ingestion at most this often
     *
     * @var int|null
     */
    protected $sampleEverySecs;
    /**
     * A new id every time tracing is switched on
     *
     * @var string|null
     */
    protected $windowId;
    /**
     * How long the runtime captures, counted from when it first sees this windowId
     *
     * @var int|null
     */
    protected $windowSecs;
    /**
     * The runtime never captures past this, and the platform refuses records dated after it (plus a short grace)
     *
     * @return \DateTime|null
     */
    public function getHardStop(): ?\DateTime
    {
        return $this->hardStop;
    }
    /**
     * The runtime never captures past this, and the platform refuses records dated after it (plus a short grace)
     *
     * @param \DateTime|null $hardStop
     *
     * @return self
     */
    public function setHardStop(?\DateTime $hardStop): self
    {
        $this->initialized['hardStop'] = true;
        $this->hardStop = $hardStop;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getIngestionId(): ?string
    {
        return $this->ingestionId;
    }
    /**
     * @param string|null $ingestionId
     *
     * @return self
     */
    public function setIngestionId(?string $ingestionId): self
    {
        $this->initialized['ingestionId'] = true;
        $this->ingestionId = $ingestionId;
        return $this;
    }
    /**
     * The runtime starts a capture of this ingestion at most this often
     *
     * @return int|null
     */
    public function getSampleEverySecs(): ?int
    {
        return $this->sampleEverySecs;
    }
    /**
     * The runtime starts a capture of this ingestion at most this often
     *
     * @param int|null $sampleEverySecs
     *
     * @return self
     */
    public function setSampleEverySecs(?int $sampleEverySecs): self
    {
        $this->initialized['sampleEverySecs'] = true;
        $this->sampleEverySecs = $sampleEverySecs;
        return $this;
    }
    /**
     * A new id every time tracing is switched on
     *
     * @return string|null
     */
    public function getWindowId(): ?string
    {
        return $this->windowId;
    }
    /**
     * A new id every time tracing is switched on
     *
     * @param string|null $windowId
     *
     * @return self
     */
    public function setWindowId(?string $windowId): self
    {
        $this->initialized['windowId'] = true;
        $this->windowId = $windowId;
        return $this;
    }
    /**
     * How long the runtime captures, counted from when it first sees this windowId
     *
     * @return int|null
     */
    public function getWindowSecs(): ?int
    {
        return $this->windowSecs;
    }
    /**
     * How long the runtime captures, counted from when it first sees this windowId
     *
     * @param int|null $windowSecs
     *
     * @return self
     */
    public function setWindowSecs(?int $windowSecs): self
    {
        $this->initialized['windowSecs'] = true;
        $this->windowSecs = $windowSecs;
        return $this;
    }
}