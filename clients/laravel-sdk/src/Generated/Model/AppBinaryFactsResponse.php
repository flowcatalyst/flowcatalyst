<?php

namespace FlowCatalyst\Generated\Model;

class AppBinaryFactsResponse
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
     * Signed with an Android debug keystore (certificate CN "Android Debug")
     *
     * @var bool|null
     */
    protected $debugCertificate;
    /**
     * android:debuggable on the application
     *
     * @var bool|null
     */
    protected $debuggable;
    /**
     * uses-sdk minSdkVersion; 0 when absent
     *
     * @var int|null
     */
    protected $minSdk;
    /**
     * The manifest's package (application id)
     *
     * @var string|null
     */
    protected $packageName;
    /**
     * @var string|null
     */
    protected $platform;
    /**
     * The runtime version the binary sends to the update routes (expo-updates); absent when it does not use expo-updates
     *
     * @var string|null
     */
    protected $runtimeVersion;
    /**
     * SHA-256 of the signing certificate (v3's when present, else v2's), lower-case hex
     *
     * @var string|null
     */
    protected $signingCertificateSha256;
    /**
     * @var string|null
     */
    protected $signingCertificateSubject;
    /**
     * APK signature schemes that verified (2, 3)
     *
     * @var list<int>|null
     */
    protected $signingSchemes;
    /**
     * The expo-channel-name request header the binary sends
     *
     * @var string|null
     */
    protected $updateChannel;
    /**
     * The code-signing key id the binary expects updates to be signed with
     *
     * @var string|null
     */
    protected $updateKeyId;
    /**
     * The binary's expo-updates URL
     *
     * @var string|null
     */
    protected $updateUrl;
    /**
     * android:versionCode (with versionCodeMajor in the upper 32 bits)
     *
     * @var int|null
     */
    protected $versionCode;
    /**
     * @var string|null
     */
    protected $versionName;
    /**
     * Signed with an Android debug keystore (certificate CN "Android Debug")
     *
     * @return bool|null
     */
    public function getDebugCertificate(): ?bool
    {
        return $this->debugCertificate;
    }
    /**
     * Signed with an Android debug keystore (certificate CN "Android Debug")
     *
     * @param bool|null $debugCertificate
     *
     * @return self
     */
    public function setDebugCertificate(?bool $debugCertificate): self
    {
        $this->initialized['debugCertificate'] = true;
        $this->debugCertificate = $debugCertificate;
        return $this;
    }
    /**
     * android:debuggable on the application
     *
     * @return bool|null
     */
    public function getDebuggable(): ?bool
    {
        return $this->debuggable;
    }
    /**
     * android:debuggable on the application
     *
     * @param bool|null $debuggable
     *
     * @return self
     */
    public function setDebuggable(?bool $debuggable): self
    {
        $this->initialized['debuggable'] = true;
        $this->debuggable = $debuggable;
        return $this;
    }
    /**
     * uses-sdk minSdkVersion; 0 when absent
     *
     * @return int|null
     */
    public function getMinSdk(): ?int
    {
        return $this->minSdk;
    }
    /**
     * uses-sdk minSdkVersion; 0 when absent
     *
     * @param int|null $minSdk
     *
     * @return self
     */
    public function setMinSdk(?int $minSdk): self
    {
        $this->initialized['minSdk'] = true;
        $this->minSdk = $minSdk;
        return $this;
    }
    /**
     * The manifest's package (application id)
     *
     * @return string|null
     */
    public function getPackageName(): ?string
    {
        return $this->packageName;
    }
    /**
     * The manifest's package (application id)
     *
     * @param string|null $packageName
     *
     * @return self
     */
    public function setPackageName(?string $packageName): self
    {
        $this->initialized['packageName'] = true;
        $this->packageName = $packageName;
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
     * The runtime version the binary sends to the update routes (expo-updates); absent when it does not use expo-updates
     *
     * @return string|null
     */
    public function getRuntimeVersion(): ?string
    {
        return $this->runtimeVersion;
    }
    /**
     * The runtime version the binary sends to the update routes (expo-updates); absent when it does not use expo-updates
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
     * SHA-256 of the signing certificate (v3's when present, else v2's), lower-case hex
     *
     * @return string|null
     */
    public function getSigningCertificateSha256(): ?string
    {
        return $this->signingCertificateSha256;
    }
    /**
     * SHA-256 of the signing certificate (v3's when present, else v2's), lower-case hex
     *
     * @param string|null $signingCertificateSha256
     *
     * @return self
     */
    public function setSigningCertificateSha256(?string $signingCertificateSha256): self
    {
        $this->initialized['signingCertificateSha256'] = true;
        $this->signingCertificateSha256 = $signingCertificateSha256;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getSigningCertificateSubject(): ?string
    {
        return $this->signingCertificateSubject;
    }
    /**
     * @param string|null $signingCertificateSubject
     *
     * @return self
     */
    public function setSigningCertificateSubject(?string $signingCertificateSubject): self
    {
        $this->initialized['signingCertificateSubject'] = true;
        $this->signingCertificateSubject = $signingCertificateSubject;
        return $this;
    }
    /**
     * APK signature schemes that verified (2, 3)
     *
     * @return list<int>|null
     */
    public function getSigningSchemes(): ?array
    {
        return $this->signingSchemes;
    }
    /**
     * APK signature schemes that verified (2, 3)
     *
     * @param list<int>|null $signingSchemes
     *
     * @return self
     */
    public function setSigningSchemes(?array $signingSchemes): self
    {
        $this->initialized['signingSchemes'] = true;
        $this->signingSchemes = $signingSchemes;
        return $this;
    }
    /**
     * The expo-channel-name request header the binary sends
     *
     * @return string|null
     */
    public function getUpdateChannel(): ?string
    {
        return $this->updateChannel;
    }
    /**
     * The expo-channel-name request header the binary sends
     *
     * @param string|null $updateChannel
     *
     * @return self
     */
    public function setUpdateChannel(?string $updateChannel): self
    {
        $this->initialized['updateChannel'] = true;
        $this->updateChannel = $updateChannel;
        return $this;
    }
    /**
     * The code-signing key id the binary expects updates to be signed with
     *
     * @return string|null
     */
    public function getUpdateKeyId(): ?string
    {
        return $this->updateKeyId;
    }
    /**
     * The code-signing key id the binary expects updates to be signed with
     *
     * @param string|null $updateKeyId
     *
     * @return self
     */
    public function setUpdateKeyId(?string $updateKeyId): self
    {
        $this->initialized['updateKeyId'] = true;
        $this->updateKeyId = $updateKeyId;
        return $this;
    }
    /**
     * The binary's expo-updates URL
     *
     * @return string|null
     */
    public function getUpdateUrl(): ?string
    {
        return $this->updateUrl;
    }
    /**
     * The binary's expo-updates URL
     *
     * @param string|null $updateUrl
     *
     * @return self
     */
    public function setUpdateUrl(?string $updateUrl): self
    {
        $this->initialized['updateUrl'] = true;
        $this->updateUrl = $updateUrl;
        return $this;
    }
    /**
     * android:versionCode (with versionCodeMajor in the upper 32 bits)
     *
     * @return int|null
     */
    public function getVersionCode(): ?int
    {
        return $this->versionCode;
    }
    /**
     * android:versionCode (with versionCodeMajor in the upper 32 bits)
     *
     * @param int|null $versionCode
     *
     * @return self
     */
    public function setVersionCode(?int $versionCode): self
    {
        $this->initialized['versionCode'] = true;
        $this->versionCode = $versionCode;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getVersionName(): ?string
    {
        return $this->versionName;
    }
    /**
     * @param string|null $versionName
     *
     * @return self
     */
    public function setVersionName(?string $versionName): self
    {
        $this->initialized['versionName'] = true;
        $this->versionName = $versionName;
        return $this;
    }
}