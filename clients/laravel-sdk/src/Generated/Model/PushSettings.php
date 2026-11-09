<?php

namespace FlowCatalyst\Generated\Model;

use FlowCatalyst\Generated\Runtime\AdditionalAndPatternProperties;
use FlowCatalyst\Generated\Runtime\AdditionalPropertiesInterface;
class PushSettings implements AdditionalPropertiesInterface
{
    use AdditionalAndPatternProperties;
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
     * Largest push data, in bytes (default 16,384; at most 65,536)
     *
     * @var int|null
     */
    protected $maxDataBytes;
    /**
     * Open streams that listen to the application's topics (default 20,000)
     *
     * @var int|null
     */
    protected $maxStreams;
    /**
     * Ingest answers 429 for the application until unpaused; open streams stay connected
     *
     * @var bool|null
     */
    protected $paused;
    /**
     * Burst of the publish rate (default 2,000; at least the rate)
     *
     * @var int|null
     */
    protected $publishBurst;
    /**
     * Pushes per second per gateway task (default 500)
     *
     * @var int|null
     */
    protected $publishRate;
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
     * Largest push data, in bytes (default 16,384; at most 65,536)
     *
     * @return int|null
     */
    public function getMaxDataBytes(): ?int
    {
        return $this->maxDataBytes;
    }
    /**
     * Largest push data, in bytes (default 16,384; at most 65,536)
     *
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
     * Open streams that listen to the application's topics (default 20,000)
     *
     * @return int|null
     */
    public function getMaxStreams(): ?int
    {
        return $this->maxStreams;
    }
    /**
     * Open streams that listen to the application's topics (default 20,000)
     *
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
     * Ingest answers 429 for the application until unpaused; open streams stay connected
     *
     * @return bool|null
     */
    public function getPaused(): ?bool
    {
        return $this->paused;
    }
    /**
     * Ingest answers 429 for the application until unpaused; open streams stay connected
     *
     * @param bool|null $paused
     *
     * @return self
     */
    public function setPaused(?bool $paused): self
    {
        $this->initialized['paused'] = true;
        $this->paused = $paused;
        return $this;
    }
    /**
     * Burst of the publish rate (default 2,000; at least the rate)
     *
     * @return int|null
     */
    public function getPublishBurst(): ?int
    {
        return $this->publishBurst;
    }
    /**
     * Burst of the publish rate (default 2,000; at least the rate)
     *
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
     * Pushes per second per gateway task (default 500)
     *
     * @return int|null
     */
    public function getPublishRate(): ?int
    {
        return $this->publishRate;
    }
    /**
     * Pushes per second per gateway task (default 500)
     *
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
    public function definedProperties(): array
    {
        return ['dollarSchema' => ['$schema', 'getDollarSchema', 'setDollarSchema'], 'maxDataBytes' => ['maxDataBytes', 'getMaxDataBytes', 'setMaxDataBytes'], 'maxStreams' => ['maxStreams', 'getMaxStreams', 'setMaxStreams'], 'paused' => ['paused', 'getPaused', 'setPaused'], 'publishBurst' => ['publishBurst', 'getPublishBurst', 'setPublishBurst'], 'publishRate' => ['publishRate', 'getPublishRate', 'setPublishRate']];
    }
}