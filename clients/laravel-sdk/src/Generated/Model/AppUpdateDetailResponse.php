<?php

namespace FlowCatalyst\Generated\Model;

class AppUpdateDetailResponse
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
     * @var list<AppUpdateAssetResponse>|null
     */
    protected $assets;
    /**
     * Whether the manifest carries the app config (extra.expoClient)
     *
     * @var bool|null
     */
    protected $hasExpoClient;
    /**
     * @var int|null
     */
    protected $totalBytes;
    /**
     * @var AppUpdateResponse|null
     */
    protected $update;
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
     * @return list<AppUpdateAssetResponse>|null
     */
    public function getAssets(): ?array
    {
        return $this->assets;
    }
    /**
     * @param list<AppUpdateAssetResponse>|null $assets
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
     * Whether the manifest carries the app config (extra.expoClient)
     *
     * @return bool|null
     */
    public function getHasExpoClient(): ?bool
    {
        return $this->hasExpoClient;
    }
    /**
     * Whether the manifest carries the app config (extra.expoClient)
     *
     * @param bool|null $hasExpoClient
     *
     * @return self
     */
    public function setHasExpoClient(?bool $hasExpoClient): self
    {
        $this->initialized['hasExpoClient'] = true;
        $this->hasExpoClient = $hasExpoClient;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getTotalBytes(): ?int
    {
        return $this->totalBytes;
    }
    /**
     * @param int|null $totalBytes
     *
     * @return self
     */
    public function setTotalBytes(?int $totalBytes): self
    {
        $this->initialized['totalBytes'] = true;
        $this->totalBytes = $totalBytes;
        return $this;
    }
    /**
     * @return AppUpdateResponse|null
     */
    public function getUpdate(): ?AppUpdateResponse
    {
        return $this->update;
    }
    /**
     * @param AppUpdateResponse|null $update
     *
     * @return self
     */
    public function setUpdate(?AppUpdateResponse $update): self
    {
        $this->initialized['update'] = true;
        $this->update = $update;
        return $this;
    }
}