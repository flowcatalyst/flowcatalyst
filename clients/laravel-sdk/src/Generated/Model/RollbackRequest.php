<?php

namespace FlowCatalyst\Generated\Model;

use FlowCatalyst\Generated\Runtime\AdditionalAndPatternProperties;
use FlowCatalyst\Generated\Runtime\AdditionalPropertiesInterface;
class RollbackRequest implements AdditionalPropertiesInterface
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
     * @var string|null
     */
    protected $appKey;
    /**
     * @var string|null
     */
    protected $applicationCode;
    /**
     * @var string|null
     */
    protected $channel;
    /**
     * @var string|null
     */
    protected $message;
    /**
     * @var string|null
     */
    protected $platform;
    /**
     * @var string|null
     */
    protected $runtimeVersion;
    /**
     * PREVIOUS republishes the update before the current one; EMBEDDED tells phones to run their embedded bundle
     *
     * @var string|null
     */
    protected $to;
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
     * @return string|null
     */
    public function getAppKey(): ?string
    {
        return $this->appKey;
    }
    /**
     * @param string|null $appKey
     *
     * @return self
     */
    public function setAppKey(?string $appKey): self
    {
        $this->initialized['appKey'] = true;
        $this->appKey = $appKey;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getApplicationCode(): ?string
    {
        return $this->applicationCode;
    }
    /**
     * @param string|null $applicationCode
     *
     * @return self
     */
    public function setApplicationCode(?string $applicationCode): self
    {
        $this->initialized['applicationCode'] = true;
        $this->applicationCode = $applicationCode;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getChannel(): ?string
    {
        return $this->channel;
    }
    /**
     * @param string|null $channel
     *
     * @return self
     */
    public function setChannel(?string $channel): self
    {
        $this->initialized['channel'] = true;
        $this->channel = $channel;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getMessage(): ?string
    {
        return $this->message;
    }
    /**
     * @param string|null $message
     *
     * @return self
     */
    public function setMessage(?string $message): self
    {
        $this->initialized['message'] = true;
        $this->message = $message;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getPlatform(): ?string
    {
        return $this->platform;
    }
    /**
     * @param string|null $platform
     *
     * @return self
     */
    public function setPlatform(?string $platform): self
    {
        $this->initialized['platform'] = true;
        $this->platform = $platform;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getRuntimeVersion(): ?string
    {
        return $this->runtimeVersion;
    }
    /**
     * @param string|null $runtimeVersion
     *
     * @return self
     */
    public function setRuntimeVersion(?string $runtimeVersion): self
    {
        $this->initialized['runtimeVersion'] = true;
        $this->runtimeVersion = $runtimeVersion;
        return $this;
    }
    /**
     * PREVIOUS republishes the update before the current one; EMBEDDED tells phones to run their embedded bundle
     *
     * @return string|null
     */
    public function getTo(): ?string
    {
        return $this->to;
    }
    /**
     * PREVIOUS republishes the update before the current one; EMBEDDED tells phones to run their embedded bundle
     *
     * @param string|null $to
     *
     * @return self
     */
    public function setTo(?string $to): self
    {
        $this->initialized['to'] = true;
        $this->to = $to;
        return $this;
    }
    public function definedProperties(): array
    {
        return ['dollarSchema' => ['$schema', 'getDollarSchema', 'setDollarSchema'], 'appKey' => ['appKey', 'getAppKey', 'setAppKey'], 'applicationCode' => ['applicationCode', 'getApplicationCode', 'setApplicationCode'], 'channel' => ['channel', 'getChannel', 'setChannel'], 'message' => ['message', 'getMessage', 'setMessage'], 'platform' => ['platform', 'getPlatform', 'setPlatform'], 'runtimeVersion' => ['runtimeVersion', 'getRuntimeVersion', 'setRuntimeVersion'], 'to' => ['to', 'getTo', 'setTo']];
    }
}