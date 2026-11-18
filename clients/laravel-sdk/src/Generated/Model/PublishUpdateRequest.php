<?php

namespace FlowCatalyst\Generated\Model;

use FlowCatalyst\Generated\Runtime\AdditionalAndPatternProperties;
use FlowCatalyst\Generated\Runtime\AdditionalPropertiesInterface;
class PublishUpdateRequest implements AdditionalPropertiesInterface
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
     * Every other file, in the export's order
     *
     * @var list<PublishAssetRequest>|null
     */
    protected $assets;
    /**
     * The channel the app sends in its expo-channel-name header
     *
     * @var string|null
     */
    protected $channel;
    /**
     * The public app config (npx expo config --type public --json), served as the manifest's extra.expoClient
     *
     * @var mixed|null
     */
    protected $expoClient;
    /**
     * @var PublishLaunchAssetRequest|null
     */
    protected $launchAsset;
    /**
     * @var string|null
     */
    protected $message;
    /**
     * @var string|null
     */
    protected $platform;
    /**
     * Share of the branch's phones that receive it; omitted is 100
     *
     * @var int|null
     */
    protected $rolloutPercent;
    /**
     * The runtime version of the binaries this update is for
     *
     * @var string|null
     */
    protected $runtimeVersion;
    /**
     * @var string|null
     */
    protected $sourceCommit;
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
     * Every other file, in the export's order
     *
     * @return list<PublishAssetRequest>|null
     */
    public function getAssets(): ?array
    {
        return $this->assets;
    }
    /**
     * Every other file, in the export's order
     *
     * @param list<PublishAssetRequest>|null $assets
     *
     * @return self
     */
    public function setAssets(?array $assets): self
    {
        $this->initialized['assets'] = true;
        $this->assets = $assets;
        return $this;
    }
    /**
     * The channel the app sends in its expo-channel-name header
     *
     * @return string|null
     */
    public function getChannel(): ?string
    {
        return $this->channel;
    }
    /**
     * The channel the app sends in its expo-channel-name header
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
     * The public app config (npx expo config --type public --json), served as the manifest's extra.expoClient
     *
     * @return mixed
     */
    public function getExpoClient()
    {
        return $this->expoClient;
    }
    /**
     * The public app config (npx expo config --type public --json), served as the manifest's extra.expoClient
     *
     * @param mixed $expoClient
     *
     * @return self
     */
    public function setExpoClient($expoClient): self
    {
        $this->initialized['expoClient'] = true;
        $this->expoClient = $expoClient;
        return $this;
    }
    /**
     * @return PublishLaunchAssetRequest|null
     */
    public function getLaunchAsset(): ?PublishLaunchAssetRequest
    {
        return $this->launchAsset;
    }
    /**
     * @param PublishLaunchAssetRequest|null $launchAsset
     *
     * @return self
     */
    public function setLaunchAsset(?PublishLaunchAssetRequest $launchAsset): self
    {
        $this->initialized['launchAsset'] = true;
        $this->launchAsset = $launchAsset;
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
     * Share of the branch's phones that receive it; omitted is 100
     *
     * @return int|null
     */
    public function getRolloutPercent(): ?int
    {
        return $this->rolloutPercent;
    }
    /**
     * Share of the branch's phones that receive it; omitted is 100
     *
     * @param int|null $rolloutPercent
     *
     * @return self
     */
    public function setRolloutPercent(?int $rolloutPercent): self
    {
        $this->initialized['rolloutPercent'] = true;
        $this->rolloutPercent = $rolloutPercent;
        return $this;
    }
    /**
     * The runtime version of the binaries this update is for
     *
     * @return string|null
     */
    public function getRuntimeVersion(): ?string
    {
        return $this->runtimeVersion;
    }
    /**
     * The runtime version of the binaries this update is for
     *
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
    public function definedProperties(): array
    {
        return ['dollarSchema' => ['$schema', 'getDollarSchema', 'setDollarSchema'], 'appKey' => ['appKey', 'getAppKey', 'setAppKey'], 'applicationCode' => ['applicationCode', 'getApplicationCode', 'setApplicationCode'], 'assets' => ['assets', 'getAssets', 'setAssets'], 'channel' => ['channel', 'getChannel', 'setChannel'], 'expoClient' => ['expoClient', 'getExpoClient', 'setExpoClient'], 'launchAsset' => ['launchAsset', 'getLaunchAsset', 'setLaunchAsset'], 'message' => ['message', 'getMessage', 'setMessage'], 'platform' => ['platform', 'getPlatform', 'setPlatform'], 'rolloutPercent' => ['rolloutPercent', 'getRolloutPercent', 'setRolloutPercent'], 'runtimeVersion' => ['runtimeVersion', 'getRuntimeVersion', 'setRuntimeVersion'], 'sourceCommit' => ['sourceCommit', 'getSourceCommit', 'setSourceCommit']];
    }
}