<?php

namespace FlowCatalyst\Generated\Model;

use FlowCatalyst\Generated\Runtime\AdditionalAndPatternProperties;
use FlowCatalyst\Generated\Runtime\AdditionalPropertiesInterface;
class PublishVersionRequest implements AdditionalPropertiesInterface
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
     * Lowercase hex sha256 of a previously-uploaded artifact
     *
     * @var string|null
     */
    protected $digest;
    /**
     * "wasm" (default), "js" or "jvm" (a jar)
     *
     * @var string|null
     */
    protected $runtime;
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
     * Lowercase hex sha256 of a previously-uploaded artifact
     *
     * @return string|null
     */
    public function getDigest(): ?string
    {
        return $this->digest;
    }
    /**
     * Lowercase hex sha256 of a previously-uploaded artifact
     *
     * @param string|null $digest
     *
     * @return self
     */
    public function setDigest(?string $digest): self
    {
        $this->initialized['digest'] = true;
        $this->digest = $digest;
        return $this;
    }
    /**
     * "wasm" (default), "js" or "jvm" (a jar)
     *
     * @return string|null
     */
    public function getRuntime(): ?string
    {
        return $this->runtime;
    }
    /**
     * "wasm" (default), "js" or "jvm" (a jar)
     *
     * @param string|null $runtime
     *
     * @return self
     */
    public function setRuntime(?string $runtime): self
    {
        $this->initialized['runtime'] = true;
        $this->runtime = $runtime;
        return $this;
    }
    public function definedProperties(): array
    {
        return ['dollarSchema' => ['$schema', 'getDollarSchema', 'setDollarSchema'], 'digest' => ['digest', 'getDigest', 'setDigest'], 'runtime' => ['runtime', 'getRuntime', 'setRuntime']];
    }
}