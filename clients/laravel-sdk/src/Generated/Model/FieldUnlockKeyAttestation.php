<?php

namespace FlowCatalyst\Generated\Model;

class FieldUnlockKeyAttestation
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
    protected $attestationSecurityLevel;
    /**
     * @var int|null
     */
    protected $attestationVersion;
    /**
     * Secure hardware requires user authentication for every use of the key
     *
     * @var bool|null
     */
    protected $authPerUse;
    /**
     * Only a strong biometric unlocks the key (no device PIN or pattern)
     *
     * @var bool|null
     */
    protected $biometricOnly;
    /**
     * The attestation was made for the session that registered the key
     *
     * @var bool|null
     */
    protected $challengeMatches;
    /**
     * When the chain was checked (the registration)
     *
     * @var \DateTime|null
     */
    protected $checkedAt;
    /**
     * The chain's certificates were within their validity at registration (checked once)
     *
     * @var string|null
     */
    protected $dates;
    /**
     * @var string|null
     */
    protected $datesProblem;
    /**
     * @var string|null
     */
    protected $format;
    /**
     * @var bool|null
     */
    protected $meetsRequirement;
    /**
     * Why a chain is MALFORMED
     *
     * @var string|null
     */
    protected $problem;
    /**
     * How the attestation key reached the phone: FACTORY (expired certificates still trusted) or REMOTE (Remote Key Provisioning, dates always checked)
     *
     * @var string|null
     */
    protected $provisioning;
    /**
     * What the attestation key status list said at registration
     *
     * @var string|null
     */
    protected $revocation;
    /**
     * Hex SHA-256 of the chain root's public key
     *
     * @var string|null
     */
    protected $rootFingerprint;
    /**
     * Where Android says the key lives
     *
     * @var string|null
     */
    protected $securityLevel;
    /**
     * A P-256 signing key in secure hardware
     *
     * @var bool|null
     */
    protected $signP256;
    /**
     * NONE: no chain was sent (iOS has none). MALFORMED: the chain could not be checked. UNTRUSTED: well formed, root not configured. TRUSTED: the root is one of FC_FIELD_ATTESTATION_ROOTS
     *
     * @var string|null
     */
    protected $verdict;
    /**
     * @return string|null
     */
    public function getAttestationSecurityLevel(): ?string
    {
        return $this->attestationSecurityLevel;
    }
    /**
     * @param string|null $attestationSecurityLevel
     *
     * @return self
     */
    public function setAttestationSecurityLevel(?string $attestationSecurityLevel): self
    {
        $this->initialized['attestationSecurityLevel'] = true;
        $this->attestationSecurityLevel = $attestationSecurityLevel;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getAttestationVersion(): ?int
    {
        return $this->attestationVersion;
    }
    /**
     * @param int|null $attestationVersion
     *
     * @return self
     */
    public function setAttestationVersion(?int $attestationVersion): self
    {
        $this->initialized['attestationVersion'] = true;
        $this->attestationVersion = $attestationVersion;
        return $this;
    }
    /**
     * Secure hardware requires user authentication for every use of the key
     *
     * @return bool|null
     */
    public function getAuthPerUse(): ?bool
    {
        return $this->authPerUse;
    }
    /**
     * Secure hardware requires user authentication for every use of the key
     *
     * @param bool|null $authPerUse
     *
     * @return self
     */
    public function setAuthPerUse(?bool $authPerUse): self
    {
        $this->initialized['authPerUse'] = true;
        $this->authPerUse = $authPerUse;
        return $this;
    }
    /**
     * Only a strong biometric unlocks the key (no device PIN or pattern)
     *
     * @return bool|null
     */
    public function getBiometricOnly(): ?bool
    {
        return $this->biometricOnly;
    }
    /**
     * Only a strong biometric unlocks the key (no device PIN or pattern)
     *
     * @param bool|null $biometricOnly
     *
     * @return self
     */
    public function setBiometricOnly(?bool $biometricOnly): self
    {
        $this->initialized['biometricOnly'] = true;
        $this->biometricOnly = $biometricOnly;
        return $this;
    }
    /**
     * The attestation was made for the session that registered the key
     *
     * @return bool|null
     */
    public function getChallengeMatches(): ?bool
    {
        return $this->challengeMatches;
    }
    /**
     * The attestation was made for the session that registered the key
     *
     * @param bool|null $challengeMatches
     *
     * @return self
     */
    public function setChallengeMatches(?bool $challengeMatches): self
    {
        $this->initialized['challengeMatches'] = true;
        $this->challengeMatches = $challengeMatches;
        return $this;
    }
    /**
     * When the chain was checked (the registration)
     *
     * @return \DateTime|null
     */
    public function getCheckedAt(): ?\DateTime
    {
        return $this->checkedAt;
    }
    /**
     * When the chain was checked (the registration)
     *
     * @param \DateTime|null $checkedAt
     *
     * @return self
     */
    public function setCheckedAt(?\DateTime $checkedAt): self
    {
        $this->initialized['checkedAt'] = true;
        $this->checkedAt = $checkedAt;
        return $this;
    }
    /**
     * The chain's certificates were within their validity at registration (checked once)
     *
     * @return string|null
     */
    public function getDates(): ?string
    {
        return $this->dates;
    }
    /**
     * The chain's certificates were within their validity at registration (checked once)
     *
     * @param string|null $dates
     *
     * @return self
     */
    public function setDates(?string $dates): self
    {
        $this->initialized['dates'] = true;
        $this->dates = $dates;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getDatesProblem(): ?string
    {
        return $this->datesProblem;
    }
    /**
     * @param string|null $datesProblem
     *
     * @return self
     */
    public function setDatesProblem(?string $datesProblem): self
    {
        $this->initialized['datesProblem'] = true;
        $this->datesProblem = $datesProblem;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getFormat(): ?string
    {
        return $this->format;
    }
    /**
     * @param string|null $format
     *
     * @return self
     */
    public function setFormat(?string $format): self
    {
        $this->initialized['format'] = true;
        $this->format = $format;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getMeetsRequirement(): ?bool
    {
        return $this->meetsRequirement;
    }
    /**
     * @param bool|null $meetsRequirement
     *
     * @return self
     */
    public function setMeetsRequirement(?bool $meetsRequirement): self
    {
        $this->initialized['meetsRequirement'] = true;
        $this->meetsRequirement = $meetsRequirement;
        return $this;
    }
    /**
     * Why a chain is MALFORMED
     *
     * @return string|null
     */
    public function getProblem(): ?string
    {
        return $this->problem;
    }
    /**
     * Why a chain is MALFORMED
     *
     * @param string|null $problem
     *
     * @return self
     */
    public function setProblem(?string $problem): self
    {
        $this->initialized['problem'] = true;
        $this->problem = $problem;
        return $this;
    }
    /**
     * How the attestation key reached the phone: FACTORY (expired certificates still trusted) or REMOTE (Remote Key Provisioning, dates always checked)
     *
     * @return string|null
     */
    public function getProvisioning(): ?string
    {
        return $this->provisioning;
    }
    /**
     * How the attestation key reached the phone: FACTORY (expired certificates still trusted) or REMOTE (Remote Key Provisioning, dates always checked)
     *
     * @param string|null $provisioning
     *
     * @return self
     */
    public function setProvisioning(?string $provisioning): self
    {
        $this->initialized['provisioning'] = true;
        $this->provisioning = $provisioning;
        return $this;
    }
    /**
     * What the attestation key status list said at registration
     *
     * @return string|null
     */
    public function getRevocation(): ?string
    {
        return $this->revocation;
    }
    /**
     * What the attestation key status list said at registration
     *
     * @param string|null $revocation
     *
     * @return self
     */
    public function setRevocation(?string $revocation): self
    {
        $this->initialized['revocation'] = true;
        $this->revocation = $revocation;
        return $this;
    }
    /**
     * Hex SHA-256 of the chain root's public key
     *
     * @return string|null
     */
    public function getRootFingerprint(): ?string
    {
        return $this->rootFingerprint;
    }
    /**
     * Hex SHA-256 of the chain root's public key
     *
     * @param string|null $rootFingerprint
     *
     * @return self
     */
    public function setRootFingerprint(?string $rootFingerprint): self
    {
        $this->initialized['rootFingerprint'] = true;
        $this->rootFingerprint = $rootFingerprint;
        return $this;
    }
    /**
     * Where Android says the key lives
     *
     * @return string|null
     */
    public function getSecurityLevel(): ?string
    {
        return $this->securityLevel;
    }
    /**
     * Where Android says the key lives
     *
     * @param string|null $securityLevel
     *
     * @return self
     */
    public function setSecurityLevel(?string $securityLevel): self
    {
        $this->initialized['securityLevel'] = true;
        $this->securityLevel = $securityLevel;
        return $this;
    }
    /**
     * A P-256 signing key in secure hardware
     *
     * @return bool|null
     */
    public function getSignP256(): ?bool
    {
        return $this->signP256;
    }
    /**
     * A P-256 signing key in secure hardware
     *
     * @param bool|null $signP256
     *
     * @return self
     */
    public function setSignP256(?bool $signP256): self
    {
        $this->initialized['signP256'] = true;
        $this->signP256 = $signP256;
        return $this;
    }
    /**
     * NONE: no chain was sent (iOS has none). MALFORMED: the chain could not be checked. UNTRUSTED: well formed, root not configured. TRUSTED: the root is one of FC_FIELD_ATTESTATION_ROOTS
     *
     * @return string|null
     */
    public function getVerdict(): ?string
    {
        return $this->verdict;
    }
    /**
     * NONE: no chain was sent (iOS has none). MALFORMED: the chain could not be checked. UNTRUSTED: well formed, root not configured. TRUSTED: the root is one of FC_FIELD_ATTESTATION_ROOTS
     *
     * @param string|null $verdict
     *
     * @return self
     */
    public function setVerdict(?string $verdict): self
    {
        $this->initialized['verdict'] = true;
        $this->verdict = $verdict;
        return $this;
    }
}