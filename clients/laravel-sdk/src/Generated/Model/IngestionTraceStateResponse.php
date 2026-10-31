<?php

namespace FlowCatalyst\Generated\Model;

class IngestionTraceStateResponse
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
     * @var \DateTime|null
     */
    protected $enabledAt;
    /**
     * @var string|null
     */
    protected $enabledBy;
    /**
     * No runtime captures past this; switching off sets it to the switch-off time
     *
     * @var \DateTime|null
     */
    protected $hardStop;
    /**
     * The window is open now (hardStop is in the future)
     *
     * @var bool|null
     */
    protected $live;
    /**
     * The longest window that may be switched on
     *
     * @var int|null
     */
    protected $maxWindowSecs;
    /**
     * A runtime starts a capture of this ingestion at most this often
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
     * How long each runtime captures, counted from when it first sees the window
     *
     * @var int|null
     */
    protected $windowSecs;
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
     * @return \DateTime|null
     */
    public function getEnabledAt(): ?\DateTime
    {
        return $this->enabledAt;
    }
    /**
     * @param \DateTime|null $enabledAt
     *
     * @return self
     */
    public function setEnabledAt(?\DateTime $enabledAt): self
    {
        $this->initialized['enabledAt'] = true;
        $this->enabledAt = $enabledAt;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getEnabledBy(): ?string
    {
        return $this->enabledBy;
    }
    /**
     * @param string|null $enabledBy
     *
     * @return self
     */
    public function setEnabledBy(?string $enabledBy): self
    {
        $this->initialized['enabledBy'] = true;
        $this->enabledBy = $enabledBy;
        return $this;
    }
    /**
     * No runtime captures past this; switching off sets it to the switch-off time
     *
     * @return \DateTime|null
     */
    public function getHardStop(): ?\DateTime
    {
        return $this->hardStop;
    }
    /**
     * No runtime captures past this; switching off sets it to the switch-off time
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
     * The window is open now (hardStop is in the future)
     *
     * @return bool|null
     */
    public function getLive(): ?bool
    {
        return $this->live;
    }
    /**
     * The window is open now (hardStop is in the future)
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
     * The longest window that may be switched on
     *
     * @return int|null
     */
    public function getMaxWindowSecs(): ?int
    {
        return $this->maxWindowSecs;
    }
    /**
     * The longest window that may be switched on
     *
     * @param int|null $maxWindowSecs
     *
     * @return self
     */
    public function setMaxWindowSecs(?int $maxWindowSecs): self
    {
        $this->initialized['maxWindowSecs'] = true;
        $this->maxWindowSecs = $maxWindowSecs;
        return $this;
    }
    /**
     * A runtime starts a capture of this ingestion at most this often
     *
     * @return int|null
     */
    public function getSampleEverySecs(): ?int
    {
        return $this->sampleEverySecs;
    }
    /**
     * A runtime starts a capture of this ingestion at most this often
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
     * How long each runtime captures, counted from when it first sees the window
     *
     * @return int|null
     */
    public function getWindowSecs(): ?int
    {
        return $this->windowSecs;
    }
    /**
     * How long each runtime captures, counted from when it first sees the window
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