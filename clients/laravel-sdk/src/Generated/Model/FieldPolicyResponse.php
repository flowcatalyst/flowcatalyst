<?php

namespace FlowCatalyst\Generated\Model;

class FieldPolicyResponse
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
     * @var string|null
     */
    protected $clientId;
    /**
     * @var int|null
     */
    protected $hardLockThreshold;
    /**
     * @var bool|null
     */
    protected $isDefault;
    /**
     * @var int|null
     */
    protected $personalIdleSecs;
    /**
     * @var int|null
     */
    protected $pinMinLength;
    /**
     * @var int|null
     */
    protected $pinReauthDays;
    /**
     * @var bool|null
     */
    protected $requireAttestation;
    /**
     * @var int|null
     */
    protected $sharedIdleSecs;
    /**
     * @var int|null
     */
    protected $shiftMaxSecs;
    /**
     * @var int|null
     */
    protected $softLockSecs;
    /**
     * @var int|null
     */
    protected $softLockThreshold;
    /**
     * @var \DateTime|null
     */
    protected $updatedAt;
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
    public function getClientId(): ?string
    {
        return $this->clientId;
    }
    /**
     * @param string|null $clientId
     *
     * @return self
     */
    public function setClientId(?string $clientId): self
    {
        $this->initialized['clientId'] = true;
        $this->clientId = $clientId;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getHardLockThreshold(): ?int
    {
        return $this->hardLockThreshold;
    }
    /**
     * @param int|null $hardLockThreshold
     *
     * @return self
     */
    public function setHardLockThreshold(?int $hardLockThreshold): self
    {
        $this->initialized['hardLockThreshold'] = true;
        $this->hardLockThreshold = $hardLockThreshold;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getIsDefault(): ?bool
    {
        return $this->isDefault;
    }
    /**
     * @param bool|null $isDefault
     *
     * @return self
     */
    public function setIsDefault(?bool $isDefault): self
    {
        $this->initialized['isDefault'] = true;
        $this->isDefault = $isDefault;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getPersonalIdleSecs(): ?int
    {
        return $this->personalIdleSecs;
    }
    /**
     * @param int|null $personalIdleSecs
     *
     * @return self
     */
    public function setPersonalIdleSecs(?int $personalIdleSecs): self
    {
        $this->initialized['personalIdleSecs'] = true;
        $this->personalIdleSecs = $personalIdleSecs;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getPinMinLength(): ?int
    {
        return $this->pinMinLength;
    }
    /**
     * @param int|null $pinMinLength
     *
     * @return self
     */
    public function setPinMinLength(?int $pinMinLength): self
    {
        $this->initialized['pinMinLength'] = true;
        $this->pinMinLength = $pinMinLength;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getPinReauthDays(): ?int
    {
        return $this->pinReauthDays;
    }
    /**
     * @param int|null $pinReauthDays
     *
     * @return self
     */
    public function setPinReauthDays(?int $pinReauthDays): self
    {
        $this->initialized['pinReauthDays'] = true;
        $this->pinReauthDays = $pinReauthDays;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getRequireAttestation(): ?bool
    {
        return $this->requireAttestation;
    }
    /**
     * @param bool|null $requireAttestation
     *
     * @return self
     */
    public function setRequireAttestation(?bool $requireAttestation): self
    {
        $this->initialized['requireAttestation'] = true;
        $this->requireAttestation = $requireAttestation;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getSharedIdleSecs(): ?int
    {
        return $this->sharedIdleSecs;
    }
    /**
     * @param int|null $sharedIdleSecs
     *
     * @return self
     */
    public function setSharedIdleSecs(?int $sharedIdleSecs): self
    {
        $this->initialized['sharedIdleSecs'] = true;
        $this->sharedIdleSecs = $sharedIdleSecs;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getShiftMaxSecs(): ?int
    {
        return $this->shiftMaxSecs;
    }
    /**
     * @param int|null $shiftMaxSecs
     *
     * @return self
     */
    public function setShiftMaxSecs(?int $shiftMaxSecs): self
    {
        $this->initialized['shiftMaxSecs'] = true;
        $this->shiftMaxSecs = $shiftMaxSecs;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getSoftLockSecs(): ?int
    {
        return $this->softLockSecs;
    }
    /**
     * @param int|null $softLockSecs
     *
     * @return self
     */
    public function setSoftLockSecs(?int $softLockSecs): self
    {
        $this->initialized['softLockSecs'] = true;
        $this->softLockSecs = $softLockSecs;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getSoftLockThreshold(): ?int
    {
        return $this->softLockThreshold;
    }
    /**
     * @param int|null $softLockThreshold
     *
     * @return self
     */
    public function setSoftLockThreshold(?int $softLockThreshold): self
    {
        $this->initialized['softLockThreshold'] = true;
        $this->softLockThreshold = $softLockThreshold;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getUpdatedAt(): ?\DateTime
    {
        return $this->updatedAt;
    }
    /**
     * @param \DateTime|null $updatedAt
     *
     * @return self
     */
    public function setUpdatedAt(?\DateTime $updatedAt): self
    {
        $this->initialized['updatedAt'] = true;
        $this->updatedAt = $updatedAt;
        return $this;
    }
}