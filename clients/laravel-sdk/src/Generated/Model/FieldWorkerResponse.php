<?php

namespace FlowCatalyst\Generated\Model;

class FieldWorkerResponse
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
     * @var list<FieldWorkerAppRef>|null
     */
    protected $apps;
    /**
     * @var FieldWorkerBadgeRef|null
     */
    protected $badge;
    /**
     * @var bool|null
     */
    protected $canImpersonate;
    /**
     * @var string|null
     */
    protected $clientId;
    /**
     * @var bool|null
     */
    protected $created;
    /**
     * @var \DateTime|null
     */
    protected $createdAt;
    /**
     * @var string|null
     */
    protected $email;
    /**
     * @var int|null
     */
    protected $failedSinceSuccess;
    /**
     * @var bool|null
     */
    protected $hardLocked;
    /**
     * @var bool|null
     */
    protected $hasPin;
    /**
     * @var string|null
     */
    protected $id;
    /**
     * @var bool|null
     */
    protected $isTest;
    /**
     * @var \DateTime|null
     */
    protected $lastLoginAt;
    /**
     * @var FieldLinkedPrincipalRef|null
     */
    protected $linkedPrincipal;
    /**
     * @var bool|null
     */
    protected $locked;
    /**
     * @var \DateTime|null
     */
    protected $lockedUntil;
    /**
     * @var string|null
     */
    protected $name;
    /**
     * @var string|null
     */
    protected $phone;
    /**
     * @var bool|null
     */
    protected $pinChangeRequired;
    /**
     * @var string|null
     */
    protected $status;
    /**
     * @var \DateTime|null
     */
    protected $updatedAt;
    /**
     * @var string|null
     */
    protected $workerCode;
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
     * @return list<FieldWorkerAppRef>|null
     */
    public function getApps(): ?array
    {
        return $this->apps;
    }
    /**
     * @param list<FieldWorkerAppRef>|null $apps
     *
     * @return self
     */
    public function setApps(?array $apps): self
    {
        $this->initialized['apps'] = true;
        $this->apps = $apps;
        return $this;
    }
    /**
     * @return FieldWorkerBadgeRef|null
     */
    public function getBadge(): ?FieldWorkerBadgeRef
    {
        return $this->badge;
    }
    /**
     * @param FieldWorkerBadgeRef|null $badge
     *
     * @return self
     */
    public function setBadge(?FieldWorkerBadgeRef $badge): self
    {
        $this->initialized['badge'] = true;
        $this->badge = $badge;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getCanImpersonate(): ?bool
    {
        return $this->canImpersonate;
    }
    /**
     * @param bool|null $canImpersonate
     *
     * @return self
     */
    public function setCanImpersonate(?bool $canImpersonate): self
    {
        $this->initialized['canImpersonate'] = true;
        $this->canImpersonate = $canImpersonate;
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
     * @return bool|null
     */
    public function getCreated(): ?bool
    {
        return $this->created;
    }
    /**
     * @param bool|null $created
     *
     * @return self
     */
    public function setCreated(?bool $created): self
    {
        $this->initialized['created'] = true;
        $this->created = $created;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getCreatedAt(): ?\DateTime
    {
        return $this->createdAt;
    }
    /**
     * @param \DateTime|null $createdAt
     *
     * @return self
     */
    public function setCreatedAt(?\DateTime $createdAt): self
    {
        $this->initialized['createdAt'] = true;
        $this->createdAt = $createdAt;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getEmail(): ?string
    {
        return $this->email;
    }
    /**
     * @param string|null $email
     *
     * @return self
     */
    public function setEmail(?string $email): self
    {
        $this->initialized['email'] = true;
        $this->email = $email;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getFailedSinceSuccess(): ?int
    {
        return $this->failedSinceSuccess;
    }
    /**
     * @param int|null $failedSinceSuccess
     *
     * @return self
     */
    public function setFailedSinceSuccess(?int $failedSinceSuccess): self
    {
        $this->initialized['failedSinceSuccess'] = true;
        $this->failedSinceSuccess = $failedSinceSuccess;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getHardLocked(): ?bool
    {
        return $this->hardLocked;
    }
    /**
     * @param bool|null $hardLocked
     *
     * @return self
     */
    public function setHardLocked(?bool $hardLocked): self
    {
        $this->initialized['hardLocked'] = true;
        $this->hardLocked = $hardLocked;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getHasPin(): ?bool
    {
        return $this->hasPin;
    }
    /**
     * @param bool|null $hasPin
     *
     * @return self
     */
    public function setHasPin(?bool $hasPin): self
    {
        $this->initialized['hasPin'] = true;
        $this->hasPin = $hasPin;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getId(): ?string
    {
        return $this->id;
    }
    /**
     * @param string|null $id
     *
     * @return self
     */
    public function setId(?string $id): self
    {
        $this->initialized['id'] = true;
        $this->id = $id;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getIsTest(): ?bool
    {
        return $this->isTest;
    }
    /**
     * @param bool|null $isTest
     *
     * @return self
     */
    public function setIsTest(?bool $isTest): self
    {
        $this->initialized['isTest'] = true;
        $this->isTest = $isTest;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getLastLoginAt(): ?\DateTime
    {
        return $this->lastLoginAt;
    }
    /**
     * @param \DateTime|null $lastLoginAt
     *
     * @return self
     */
    public function setLastLoginAt(?\DateTime $lastLoginAt): self
    {
        $this->initialized['lastLoginAt'] = true;
        $this->lastLoginAt = $lastLoginAt;
        return $this;
    }
    /**
     * @return FieldLinkedPrincipalRef|null
     */
    public function getLinkedPrincipal(): ?FieldLinkedPrincipalRef
    {
        return $this->linkedPrincipal;
    }
    /**
     * @param FieldLinkedPrincipalRef|null $linkedPrincipal
     *
     * @return self
     */
    public function setLinkedPrincipal(?FieldLinkedPrincipalRef $linkedPrincipal): self
    {
        $this->initialized['linkedPrincipal'] = true;
        $this->linkedPrincipal = $linkedPrincipal;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getLocked(): ?bool
    {
        return $this->locked;
    }
    /**
     * @param bool|null $locked
     *
     * @return self
     */
    public function setLocked(?bool $locked): self
    {
        $this->initialized['locked'] = true;
        $this->locked = $locked;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getLockedUntil(): ?\DateTime
    {
        return $this->lockedUntil;
    }
    /**
     * @param \DateTime|null $lockedUntil
     *
     * @return self
     */
    public function setLockedUntil(?\DateTime $lockedUntil): self
    {
        $this->initialized['lockedUntil'] = true;
        $this->lockedUntil = $lockedUntil;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getName(): ?string
    {
        return $this->name;
    }
    /**
     * @param string|null $name
     *
     * @return self
     */
    public function setName(?string $name): self
    {
        $this->initialized['name'] = true;
        $this->name = $name;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getPhone(): ?string
    {
        return $this->phone;
    }
    /**
     * @param string|null $phone
     *
     * @return self
     */
    public function setPhone(?string $phone): self
    {
        $this->initialized['phone'] = true;
        $this->phone = $phone;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getPinChangeRequired(): ?bool
    {
        return $this->pinChangeRequired;
    }
    /**
     * @param bool|null $pinChangeRequired
     *
     * @return self
     */
    public function setPinChangeRequired(?bool $pinChangeRequired): self
    {
        $this->initialized['pinChangeRequired'] = true;
        $this->pinChangeRequired = $pinChangeRequired;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getStatus(): ?string
    {
        return $this->status;
    }
    /**
     * @param string|null $status
     *
     * @return self
     */
    public function setStatus(?string $status): self
    {
        $this->initialized['status'] = true;
        $this->status = $status;
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
    /**
     * @return string|null
     */
    public function getWorkerCode(): ?string
    {
        return $this->workerCode;
    }
    /**
     * @param string|null $workerCode
     *
     * @return self
     */
    public function setWorkerCode(?string $workerCode): self
    {
        $this->initialized['workerCode'] = true;
        $this->workerCode = $workerCode;
        return $this;
    }
}