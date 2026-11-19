<?php

namespace FlowCatalyst\Generated\Model;

use FlowCatalyst\Generated\Runtime\AdditionalAndPatternProperties;
use FlowCatalyst\Generated\Runtime\AdditionalPropertiesInterface;
class PublishBinaryRequest implements AdditionalPropertiesInterface
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
     * The phone app (lower-case letters, digits, hyphens; at most 40)
     *
     * @var string|null
     */
    protected $appKey;
    /**
     * @var string|null
     */
    protected $applicationCode;
    /**
     * The channel; when the binary names its expo-channel-name it must be this
     *
     * @var string|null
     */
    protected $channel;
    /**
     * @var string|null
     */
    protected $message;
    /**
     * Only android: an iOS app cannot be installed from a link
     *
     * @var string|null
     */
    protected $platform;
    /**
     * Lower-case hex SHA-256 of the uploaded file
     *
     * @var string|null
     */
    protected $sha256;
    /**
     * @var string|null
     */
    protected $sourceCommit;
    /**
     * release (the build real users get: not debuggable, not signed with a debug keystore), debug, or another name
     *
     * @var string|null
     */
    protected $variant;
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
     * The phone app (lower-case letters, digits, hyphens; at most 40)
     *
     * @return string|null
     */
    public function getAppKey(): ?string
    {
        return $this->appKey;
    }
    /**
     * The phone app (lower-case letters, digits, hyphens; at most 40)
     *
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
     * The channel; when the binary names its expo-channel-name it must be this
     *
     * @return string|null
     */
    public function getChannel(): ?string
    {
        return $this->channel;
    }
    /**
     * The channel; when the binary names its expo-channel-name it must be this
     *
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
     * Only android: an iOS app cannot be installed from a link
     *
     * @return string|null
     */
    public function getPlatform(): ?string
    {
        return $this->platform;
    }
    /**
     * Only android: an iOS app cannot be installed from a link
     *
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
     * Lower-case hex SHA-256 of the uploaded file
     *
     * @return string|null
     */
    public function getSha256(): ?string
    {
        return $this->sha256;
    }
    /**
     * Lower-case hex SHA-256 of the uploaded file
     *
     * @param string|null $sha256
     *
     * @return self
     */
    public function setSha256(?string $sha256): self
    {
        $this->initialized['sha256'] = true;
        $this->sha256 = $sha256;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getSourceCommit(): ?string
    {
        return $this->sourceCommit;
    }
    /**
     * @param string|null $sourceCommit
     *
     * @return self
     */
    public function setSourceCommit(?string $sourceCommit): self
    {
        $this->initialized['sourceCommit'] = true;
        $this->sourceCommit = $sourceCommit;
        return $this;
    }
    /**
     * release (the build real users get: not debuggable, not signed with a debug keystore), debug, or another name
     *
     * @return string|null
     */
    public function getVariant(): ?string
    {
        return $this->variant;
    }
    /**
     * release (the build real users get: not debuggable, not signed with a debug keystore), debug, or another name
     *
     * @param string|null $variant
     *
     * @return self
     */
    public function setVariant(?string $variant): self
    {
        $this->initialized['variant'] = true;
        $this->variant = $variant;
        return $this;
    }
    public function definedProperties(): array
    {
        return ['dollarSchema' => ['$schema', 'getDollarSchema', 'setDollarSchema'], 'appKey' => ['appKey', 'getAppKey', 'setAppKey'], 'applicationCode' => ['applicationCode', 'getApplicationCode', 'setApplicationCode'], 'channel' => ['channel', 'getChannel', 'setChannel'], 'message' => ['message', 'getMessage', 'setMessage'], 'platform' => ['platform', 'getPlatform', 'setPlatform'], 'sha256' => ['sha256', 'getSha256', 'setSha256'], 'sourceCommit' => ['sourceCommit', 'getSourceCommit', 'setSourceCommit'], 'variant' => ['variant', 'getVariant', 'setVariant']];
    }
}