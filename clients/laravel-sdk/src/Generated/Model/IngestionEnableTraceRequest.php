<?php

namespace FlowCatalyst\Generated\Model;

use FlowCatalyst\Generated\Runtime\AdditionalAndPatternProperties;
use FlowCatalyst\Generated\Runtime\AdditionalPropertiesInterface;
class IngestionEnableTraceRequest implements AdditionalPropertiesInterface
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
     * A runtime starts a capture at most this often, 1 to 60 seconds (default 5). 400 TRACE_SAMPLE_INVALID otherwise.
     *
     * @var int|null
     */
    protected $sampleEverySecs;
    /**
     * How long each runtime captures: 60, 300 or 900 (default 60). 400 TRACE_WINDOW_INVALID otherwise.
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
     * A runtime starts a capture at most this often, 1 to 60 seconds (default 5). 400 TRACE_SAMPLE_INVALID otherwise.
     *
     * @return int|null
     */
    public function getSampleEverySecs(): ?int
    {
        return $this->sampleEverySecs;
    }
    /**
     * A runtime starts a capture at most this often, 1 to 60 seconds (default 5). 400 TRACE_SAMPLE_INVALID otherwise.
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
     * How long each runtime captures: 60, 300 or 900 (default 60). 400 TRACE_WINDOW_INVALID otherwise.
     *
     * @return int|null
     */
    public function getWindowSecs(): ?int
    {
        return $this->windowSecs;
    }
    /**
     * How long each runtime captures: 60, 300 or 900 (default 60). 400 TRACE_WINDOW_INVALID otherwise.
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
    public function definedProperties(): array
    {
        return ['dollarSchema' => ['$schema', 'getDollarSchema', 'setDollarSchema'], 'sampleEverySecs' => ['sampleEverySecs', 'getSampleEverySecs', 'setSampleEverySecs'], 'windowSecs' => ['windowSecs', 'getWindowSecs', 'setWindowSecs']];
    }
}