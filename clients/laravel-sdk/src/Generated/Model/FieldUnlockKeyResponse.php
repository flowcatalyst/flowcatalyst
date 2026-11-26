<?php

namespace FlowCatalyst\Generated\Model;

class FieldUnlockKeyResponse
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
     * @var FieldUnlockKeyAttestation|null
     */
    protected $attestation;
    /**
     * @var \DateTime|null
     */
    protected $createdAt;
    /**
     * @var string|null
     */
    protected $deviceId;
    /**
     * @var string|null
     */
    protected $id;
    /**
     * @var \DateTime|null
     */
    protected $lastUsedAt;
    /**
     * Sessions this key signed in that are still live; revoking the key (also a revoked one) ends them
     *
     * @var int|null
     */
    protected $liveSessions;
    /**
     * @var \DateTime|null
     */
    protected $pinRequiredBy;
    /**
     * The last time the worker proved their PIN on the device
     *
     * @var \DateTime|null
     */
    protected $pinVerifiedAt;
    /**
     * @var \DateTime|null
     */
    protected $revokedAt;
    /**
     * @var string|null
     */
    protected $revokedBy;
    /**
     * @var string|null
     */
    protected $revokedReason;
    /**
     * @var string|null
     */
    protected $status;
    /**
     * @var string|null
     */
    protected $workerId;
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
     * @return FieldUnlockKeyAttestation|null
     */
    public function getAttestation(): ?FieldUnlockKeyAttestation
    {
        return $this->attestation;
    }
    /**
     * @param FieldUnlockKeyAttestation|null $attestation
     *
     * @return self
     */
    public function setAttestation(?FieldUnlockKeyAttestation $attestation): self
    {
        $this->initialized['attestation'] = true;
        $this->attestation = $attestation;
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
    public function getDeviceId(): ?string
    {
        return $this->deviceId;
    }
    /**
     * @param string|null $deviceId
     *
     * @return self
     */
    public function setDeviceId(?string $deviceId): self
    {
        $this->initialized['deviceId'] = true;
        $this->deviceId = $deviceId;
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
     * @return \DateTime|null
     */
    public function getLastUsedAt(): ?\DateTime
    {
        return $this->lastUsedAt;
    }
    /**
     * @param \DateTime|null $lastUsedAt
     *
     * @return self
     */
    public function setLastUsedAt(?\DateTime $lastUsedAt): self
    {
        $this->initialized['lastUsedAt'] = true;
        $this->lastUsedAt = $lastUsedAt;
        return $this;
    }
    /**
     * Sessions this key signed in that are still live; revoking the key (also a revoked one) ends them
     *
     * @return int|null
     */
    public function getLiveSessions(): ?int
    {
        return $this->liveSessions;
    }
    /**
     * Sessions this key signed in that are still live; revoking the key (also a revoked one) ends them
     *
     * @param int|null $liveSessions
     *
     * @return self
     */
    public function setLiveSessions(?int $liveSessions): self
    {
        $this->initialized['liveSessions'] = true;
        $this->liveSessions = $liveSessions;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getPinRequiredBy(): ?\DateTime
    {
        return $this->pinRequiredBy;
    }
    /**
     * @param \DateTime|null $pinRequiredBy
     *
     * @return self
     */
    public function setPinRequiredBy(?\DateTime $pinRequiredBy): self
    {
        $this->initialized['pinRequiredBy'] = true;
        $this->pinRequiredBy = $pinRequiredBy;
        return $this;
    }
    /**
     * The last time the worker proved their PIN on the device
     *
     * @return \DateTime|null
     */
    public function getPinVerifiedAt(): ?\DateTime
    {
        return $this->pinVerifiedAt;
    }
    /**
     * The last time the worker proved their PIN on the device
     *
     * @param \DateTime|null $pinVerifiedAt
     *
     * @return self
     */
    public function setPinVerifiedAt(?\DateTime $pinVerifiedAt): self
    {
        $this->initialized['pinVerifiedAt'] = true;
        $this->pinVerifiedAt = $pinVerifiedAt;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getRevokedAt(): ?\DateTime
    {
        return $this->revokedAt;
    }
    /**
     * @param \DateTime|null $revokedAt
     *
     * @return self
     */
    public function setRevokedAt(?\DateTime $revokedAt): self
    {
        $this->initialized['revokedAt'] = true;
        $this->revokedAt = $revokedAt;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getRevokedBy(): ?string
    {
        return $this->revokedBy;
    }
    /**
     * @param string|null $revokedBy
     *
     * @return self
     */
    public function setRevokedBy(?string $revokedBy): self
    {
        $this->initialized['revokedBy'] = true;
        $this->revokedBy = $revokedBy;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getRevokedReason(): ?string
    {
        return $this->revokedReason;
    }
    /**
     * @param string|null $revokedReason
     *
     * @return self
     */
    public function setRevokedReason(?string $revokedReason): self
    {
        $this->initialized['revokedReason'] = true;
        $this->revokedReason = $revokedReason;
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
     * @return string|null
     */
    public function getWorkerId(): ?string
    {
        return $this->workerId;
    }
    /**
     * @param string|null $workerId
     *
     * @return self
     */
    public function setWorkerId(?string $workerId): self
    {
        $this->initialized['workerId'] = true;
        $this->workerId = $workerId;
        return $this;
    }
}