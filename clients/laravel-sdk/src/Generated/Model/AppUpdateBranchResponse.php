<?php

namespace FlowCatalyst\Generated\Model;

class AppUpdateBranchResponse
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
    protected $appKey;
    /**
     * @var string|null
     */
    protected $channel;
    /**
     * @var AppUpdateResponse|null
     */
    protected $current;
    /**
     * @var string|null
     */
    protected $platform;
    /**
     * @var AppUpdateResponse|null
     */
    protected $previous;
    /**
     * @var string|null
     */
    protected $runtimeVersion;
    /**
     * @var int|null
     */
    protected $updateCount;
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
     * @return AppUpdateResponse|null
     */
    public function getCurrent(): ?AppUpdateResponse
    {
        return $this->current;
    }
    /**
     * @param AppUpdateResponse|null $current
     *
     * @return self
     */
    public function setCurrent(?AppUpdateResponse $current): self
    {
        $this->initialized['current'] = true;
        $this->current = $current;
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
     * @return AppUpdateResponse|null
     */
    public function getPrevious(): ?AppUpdateResponse
    {
        return $this->previous;
    }
    /**
     * @param AppUpdateResponse|null $previous
     *
     * @return self
     */
    public function setPrevious(?AppUpdateResponse $previous): self
    {
        $this->initialized['previous'] = true;
        $this->previous = $previous;
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
     * @return int|null
     */
    public function getUpdateCount(): ?int
    {
        return $this->updateCount;
    }
    /**
     * @param int|null $updateCount
     *
     * @return self
     */
    public function setUpdateCount(?int $updateCount): self
    {
        $this->initialized['updateCount'] = true;
        $this->updateCount = $updateCount;
        return $this;
    }
}