<?php

namespace FlowCatalyst\Generated\Model;

use FlowCatalyst\Generated\Runtime\AdditionalAndPatternProperties;
use FlowCatalyst\Generated\Runtime\AdditionalPropertiesInterface;
class PutFieldPolicyRequest implements AdditionalPropertiesInterface
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
     * Let a badge sign in without a PIN on an enrolled device (default false)
     *
     * @var bool|null
     */
    protected $badgeOnlyLogin;
    /**
     * softLockThreshold to 50 (default 10)
     *
     * @var int|null
     */
    protected $hardLockThreshold;
    /**
     * 300 to 43200 (default 14400)
     *
     * @var int|null
     */
    protected $impersonationMaxSecs;
    /**
     * 60 to 86400 (default 7200)
     *
     * @var int|null
     */
    protected $personalIdleSecs;
    /**
     * Field-worker photos for this client (default true). False hides them: none can be set and none is shown, but the stored photos are kept (DELETE /api/field-policies/{clientId}/photos deletes them)
     *
     * @var bool|null
     */
    protected $photosEnabled;
    /**
     * 4 to 12 (default 6)
     *
     * @var int|null
     */
    protected $pinMinLength;
    /**
     * 0 to 90 (default 0): the days an unlock key signs a worker in after they last entered their PIN on the device; 0 forces no periodic PIN
     *
     * @var int|null
     */
    protected $pinReauthDays;
    /**
     * @var bool|null
     */
    protected $requireAttestation;
    /**
     * Refuse sign-in from a key that is not an enrolled device (default false)
     *
     * @var bool|null
     */
    protected $requireEnrolledDevice;
    /**
     * 60 to 86400 (default 900)
     *
     * @var int|null
     */
    protected $sharedIdleSecs;
    /**
     * 3600 to 86400 (default 50400)
     *
     * @var int|null
     */
    protected $shiftMaxSecs;
    /**
     * 30 to 86400 (default 900)
     *
     * @var int|null
     */
    protected $softLockSecs;
    /**
     * 1 to 20 (default 5)
     *
     * @var int|null
     */
    protected $softLockThreshold;
    /**
     * 60 to 86400 (default 900)
     *
     * @var int|null
     */
    protected $unenrolledIdleSecs;
    /**
     * NONE (default): an unlock key's attestation is recorded, not enforced. REQUIRED: only a key with a trusted Android key attestation (secure hardware, strong biometric for every use) may be registered; needs FC_FIELD_ATTESTATION_ROOTS, and switching to it revokes the client's keys that do not meet it
     *
     * @var string|null
     */
    protected $unlockKeyAttestation;
    /**
     * Let a worker register an unlock key on a PERSONAL device and sign in again with a fingerprint or face (default false). Switching it off revokes every unlock key of the client
     *
     * @var bool|null
     */
    protected $unlockKeysEnabled;
    /**
     * Digits after the three letters of a worker code the platform generates, 3 to 6 (default 3); applies to codes generated afterwards only
     *
     * @var int|null
     */
    protected $workerCodeDigits;
    /**
     * Let a signed-in worker set and remove their own photo from the device (default false)
     *
     * @var bool|null
     */
    protected $workerMaySetOwnPhoto;
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
     * Let a badge sign in without a PIN on an enrolled device (default false)
     *
     * @return bool|null
     */
    public function getBadgeOnlyLogin(): ?bool
    {
        return $this->badgeOnlyLogin;
    }
    /**
     * Let a badge sign in without a PIN on an enrolled device (default false)
     *
     * @param bool|null $badgeOnlyLogin
     *
     * @return self
     */
    public function setBadgeOnlyLogin(?bool $badgeOnlyLogin): self
    {
        $this->initialized['badgeOnlyLogin'] = true;
        $this->badgeOnlyLogin = $badgeOnlyLogin;
        return $this;
    }
    /**
     * softLockThreshold to 50 (default 10)
     *
     * @return int|null
     */
    public function getHardLockThreshold(): ?int
    {
        return $this->hardLockThreshold;
    }
    /**
     * softLockThreshold to 50 (default 10)
     *
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
     * 300 to 43200 (default 14400)
     *
     * @return int|null
     */
    public function getImpersonationMaxSecs(): ?int
    {
        return $this->impersonationMaxSecs;
    }
    /**
     * 300 to 43200 (default 14400)
     *
     * @param int|null $impersonationMaxSecs
     *
     * @return self
     */
    public function setImpersonationMaxSecs(?int $impersonationMaxSecs): self
    {
        $this->initialized['impersonationMaxSecs'] = true;
        $this->impersonationMaxSecs = $impersonationMaxSecs;
        return $this;
    }
    /**
     * 60 to 86400 (default 7200)
     *
     * @return int|null
     */
    public function getPersonalIdleSecs(): ?int
    {
        return $this->personalIdleSecs;
    }
    /**
     * 60 to 86400 (default 7200)
     *
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
     * Field-worker photos for this client (default true). False hides them: none can be set and none is shown, but the stored photos are kept (DELETE /api/field-policies/{clientId}/photos deletes them)
     *
     * @return bool|null
     */
    public function getPhotosEnabled(): ?bool
    {
        return $this->photosEnabled;
    }
    /**
     * Field-worker photos for this client (default true). False hides them: none can be set and none is shown, but the stored photos are kept (DELETE /api/field-policies/{clientId}/photos deletes them)
     *
     * @param bool|null $photosEnabled
     *
     * @return self
     */
    public function setPhotosEnabled(?bool $photosEnabled): self
    {
        $this->initialized['photosEnabled'] = true;
        $this->photosEnabled = $photosEnabled;
        return $this;
    }
    /**
     * 4 to 12 (default 6)
     *
     * @return int|null
     */
    public function getPinMinLength(): ?int
    {
        return $this->pinMinLength;
    }
    /**
     * 4 to 12 (default 6)
     *
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
     * 0 to 90 (default 0): the days an unlock key signs a worker in after they last entered their PIN on the device; 0 forces no periodic PIN
     *
     * @return int|null
     */
    public function getPinReauthDays(): ?int
    {
        return $this->pinReauthDays;
    }
    /**
     * 0 to 90 (default 0): the days an unlock key signs a worker in after they last entered their PIN on the device; 0 forces no periodic PIN
     *
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
     * Refuse sign-in from a key that is not an enrolled device (default false)
     *
     * @return bool|null
     */
    public function getRequireEnrolledDevice(): ?bool
    {
        return $this->requireEnrolledDevice;
    }
    /**
     * Refuse sign-in from a key that is not an enrolled device (default false)
     *
     * @param bool|null $requireEnrolledDevice
     *
     * @return self
     */
    public function setRequireEnrolledDevice(?bool $requireEnrolledDevice): self
    {
        $this->initialized['requireEnrolledDevice'] = true;
        $this->requireEnrolledDevice = $requireEnrolledDevice;
        return $this;
    }
    /**
     * 60 to 86400 (default 900)
     *
     * @return int|null
     */
    public function getSharedIdleSecs(): ?int
    {
        return $this->sharedIdleSecs;
    }
    /**
     * 60 to 86400 (default 900)
     *
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
     * 3600 to 86400 (default 50400)
     *
     * @return int|null
     */
    public function getShiftMaxSecs(): ?int
    {
        return $this->shiftMaxSecs;
    }
    /**
     * 3600 to 86400 (default 50400)
     *
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
     * 30 to 86400 (default 900)
     *
     * @return int|null
     */
    public function getSoftLockSecs(): ?int
    {
        return $this->softLockSecs;
    }
    /**
     * 30 to 86400 (default 900)
     *
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
     * 1 to 20 (default 5)
     *
     * @return int|null
     */
    public function getSoftLockThreshold(): ?int
    {
        return $this->softLockThreshold;
    }
    /**
     * 1 to 20 (default 5)
     *
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
     * 60 to 86400 (default 900)
     *
     * @return int|null
     */
    public function getUnenrolledIdleSecs(): ?int
    {
        return $this->unenrolledIdleSecs;
    }
    /**
     * 60 to 86400 (default 900)
     *
     * @param int|null $unenrolledIdleSecs
     *
     * @return self
     */
    public function setUnenrolledIdleSecs(?int $unenrolledIdleSecs): self
    {
        $this->initialized['unenrolledIdleSecs'] = true;
        $this->unenrolledIdleSecs = $unenrolledIdleSecs;
        return $this;
    }
    /**
     * NONE (default): an unlock key's attestation is recorded, not enforced. REQUIRED: only a key with a trusted Android key attestation (secure hardware, strong biometric for every use) may be registered; needs FC_FIELD_ATTESTATION_ROOTS, and switching to it revokes the client's keys that do not meet it
     *
     * @return string|null
     */
    public function getUnlockKeyAttestation(): ?string
    {
        return $this->unlockKeyAttestation;
    }
    /**
     * NONE (default): an unlock key's attestation is recorded, not enforced. REQUIRED: only a key with a trusted Android key attestation (secure hardware, strong biometric for every use) may be registered; needs FC_FIELD_ATTESTATION_ROOTS, and switching to it revokes the client's keys that do not meet it
     *
     * @param string|null $unlockKeyAttestation
     *
     * @return self
     */
    public function setUnlockKeyAttestation(?string $unlockKeyAttestation): self
    {
        $this->initialized['unlockKeyAttestation'] = true;
        $this->unlockKeyAttestation = $unlockKeyAttestation;
        return $this;
    }
    /**
     * Let a worker register an unlock key on a PERSONAL device and sign in again with a fingerprint or face (default false). Switching it off revokes every unlock key of the client
     *
     * @return bool|null
     */
    public function getUnlockKeysEnabled(): ?bool
    {
        return $this->unlockKeysEnabled;
    }
    /**
     * Let a worker register an unlock key on a PERSONAL device and sign in again with a fingerprint or face (default false). Switching it off revokes every unlock key of the client
     *
     * @param bool|null $unlockKeysEnabled
     *
     * @return self
     */
    public function setUnlockKeysEnabled(?bool $unlockKeysEnabled): self
    {
        $this->initialized['unlockKeysEnabled'] = true;
        $this->unlockKeysEnabled = $unlockKeysEnabled;
        return $this;
    }
    /**
     * Digits after the three letters of a worker code the platform generates, 3 to 6 (default 3); applies to codes generated afterwards only
     *
     * @return int|null
     */
    public function getWorkerCodeDigits(): ?int
    {
        return $this->workerCodeDigits;
    }
    /**
     * Digits after the three letters of a worker code the platform generates, 3 to 6 (default 3); applies to codes generated afterwards only
     *
     * @param int|null $workerCodeDigits
     *
     * @return self
     */
    public function setWorkerCodeDigits(?int $workerCodeDigits): self
    {
        $this->initialized['workerCodeDigits'] = true;
        $this->workerCodeDigits = $workerCodeDigits;
        return $this;
    }
    /**
     * Let a signed-in worker set and remove their own photo from the device (default false)
     *
     * @return bool|null
     */
    public function getWorkerMaySetOwnPhoto(): ?bool
    {
        return $this->workerMaySetOwnPhoto;
    }
    /**
     * Let a signed-in worker set and remove their own photo from the device (default false)
     *
     * @param bool|null $workerMaySetOwnPhoto
     *
     * @return self
     */
    public function setWorkerMaySetOwnPhoto(?bool $workerMaySetOwnPhoto): self
    {
        $this->initialized['workerMaySetOwnPhoto'] = true;
        $this->workerMaySetOwnPhoto = $workerMaySetOwnPhoto;
        return $this;
    }
    public function definedProperties(): array
    {
        return ['dollarSchema' => ['$schema', 'getDollarSchema', 'setDollarSchema'], 'badgeOnlyLogin' => ['badgeOnlyLogin', 'getBadgeOnlyLogin', 'setBadgeOnlyLogin'], 'hardLockThreshold' => ['hardLockThreshold', 'getHardLockThreshold', 'setHardLockThreshold'], 'impersonationMaxSecs' => ['impersonationMaxSecs', 'getImpersonationMaxSecs', 'setImpersonationMaxSecs'], 'personalIdleSecs' => ['personalIdleSecs', 'getPersonalIdleSecs', 'setPersonalIdleSecs'], 'photosEnabled' => ['photosEnabled', 'getPhotosEnabled', 'setPhotosEnabled'], 'pinMinLength' => ['pinMinLength', 'getPinMinLength', 'setPinMinLength'], 'pinReauthDays' => ['pinReauthDays', 'getPinReauthDays', 'setPinReauthDays'], 'requireAttestation' => ['requireAttestation', 'getRequireAttestation', 'setRequireAttestation'], 'requireEnrolledDevice' => ['requireEnrolledDevice', 'getRequireEnrolledDevice', 'setRequireEnrolledDevice'], 'sharedIdleSecs' => ['sharedIdleSecs', 'getSharedIdleSecs', 'setSharedIdleSecs'], 'shiftMaxSecs' => ['shiftMaxSecs', 'getShiftMaxSecs', 'setShiftMaxSecs'], 'softLockSecs' => ['softLockSecs', 'getSoftLockSecs', 'setSoftLockSecs'], 'softLockThreshold' => ['softLockThreshold', 'getSoftLockThreshold', 'setSoftLockThreshold'], 'unenrolledIdleSecs' => ['unenrolledIdleSecs', 'getUnenrolledIdleSecs', 'setUnenrolledIdleSecs'], 'unlockKeyAttestation' => ['unlockKeyAttestation', 'getUnlockKeyAttestation', 'setUnlockKeyAttestation'], 'unlockKeysEnabled' => ['unlockKeysEnabled', 'getUnlockKeysEnabled', 'setUnlockKeysEnabled'], 'workerCodeDigits' => ['workerCodeDigits', 'getWorkerCodeDigits', 'setWorkerCodeDigits'], 'workerMaySetOwnPhoto' => ['workerMaySetOwnPhoto', 'getWorkerMaySetOwnPhoto', 'setWorkerMaySetOwnPhoto']];
    }
}