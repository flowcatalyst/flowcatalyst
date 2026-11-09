<?php

namespace FlowCatalyst\Generated\Model;

class PushSettingsResponseEffectiveStruct
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
     * @var int|null
     */
    protected $maxDataBytes;
    /**
     * @var int|null
     */
    protected $maxStreams;
    /**
     * @var int|null
     */
    protected $publishBurst;
    /**
     * @var int|null
     */
    protected $publishRate;
    /**
     * @return int|null
     */
    public function getMaxDataBytes(): ?int
    {
        return $this->maxDataBytes;
    }
    /**
     * @param int|null $maxDataBytes
     *
     * @return self
     */
    public function setMaxDataBytes(?int $maxDataBytes): self
    {
        $this->initialized['maxDataBytes'] = true;
        $this->maxDataBytes = $maxDataBytes;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getMaxStreams(): ?int
    {
        return $this->maxStreams;
    }
    /**
     * @param int|null $maxStreams
     *
     * @return self
     */
    public function setMaxStreams(?int $maxStreams): self
    {
        $this->initialized['maxStreams'] = true;
        $this->maxStreams = $maxStreams;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getPublishBurst(): ?int
    {
        return $this->publishBurst;
    }
    /**
     * @param int|null $publishBurst
     *
     * @return self
     */
    public function setPublishBurst(?int $publishBurst): self
    {
        $this->initialized['publishBurst'] = true;
        $this->publishBurst = $publishBurst;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getPublishRate(): ?int
    {
        return $this->publishRate;
    }
    /**
     * @param int|null $publishRate
     *
     * @return self
     */
    public function setPublishRate(?int $publishRate): self
    {
        $this->initialized['publishRate'] = true;
        $this->publishRate = $publishRate;
        return $this;
    }
}