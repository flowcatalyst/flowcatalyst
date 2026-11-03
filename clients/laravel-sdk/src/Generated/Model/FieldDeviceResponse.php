<?php

namespace FlowCatalyst\Generated\Model;

class FieldDeviceResponse
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
     * @var int|null
     */
    protected $activeSessions;
    /**
     * @var list<FieldAppRef>|null
     */
    protected $apps;
    /**
     * @var string|null
     */
    protected $assignedWorkerId;
    /**
     * @var string|null
     */
    protected $attestationLevel;
    /**
     * @var string|null
     */
    protected $clientId;
    /**
     * @var \DateTime|null
     */
    protected $enrolledAt;
    /**
     * @var string|null
     */
    protected $id;
    /**
     * RFC 7638 thumbprint of the device key
     *
     * @var string|null
     */
    protected $jkt;
    /**
     * @var string|null
     */
    protected $label;
    /**
     * @var \DateTime|null
     */
    protected $lastSeenAt;
    /**
     * @var string|null
     */
    protected $mode;
    /**
     * @var string|null
     */
    protected $model;
    /**
     * @var string|null
     */
    protected $platform;
    /**
     * @var string|null
     */
    protected $status;
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
     * @return int|null
     */
    public function getActiveSessions(): ?int
    {
        return $this->activeSessions;
    }
    /**
     * @param int|null $activeSessions
     *
     * @return self
     */
    public function setActiveSessions(?int $activeSessions): self
    {
        $this->initialized['activeSessions'] = true;
        $this->activeSessions = $activeSessions;
        return $this;
    }
    /**
     * @return list<FieldAppRef>|null
     */
    public function getApps(): ?array
    {
        return $this->apps;
    }
    /**
     * @param list<FieldAppRef>|null $apps
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
     * @return string|null
     */
    public function getAssignedWorkerId(): ?string
    {
        return $this->assignedWorkerId;
    }
    /**
     * @param string|null $assignedWorkerId
     *
     * @return self
     */
    public function setAssignedWorkerId(?string $assignedWorkerId): self
    {
        $this->initialized['assignedWorkerId'] = true;
        $this->assignedWorkerId = $assignedWorkerId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getAttestationLevel(): ?string
    {
        return $this->attestationLevel;
    }
    /**
     * @param string|null $attestationLevel
     *
     * @return self
     */
    public function setAttestationLevel(?string $attestationLevel): self
    {
        $this->initialized['attestationLevel'] = true;
        $this->attestationLevel = $attestationLevel;
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
     * @return \DateTime|null
     */
    public function getEnrolledAt(): ?\DateTime
    {
        return $this->enrolledAt;
    }
    /**
     * @param \DateTime|null $enrolledAt
     *
     * @return self
     */
    public function setEnrolledAt(?\DateTime $enrolledAt): self
    {
        $this->initialized['enrolledAt'] = true;
        $this->enrolledAt = $enrolledAt;
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
     * RFC 7638 thumbprint of the device key
     *
     * @return string|null
     */
    public function getJkt(): ?string
    {
        return $this->jkt;
    }
    /**
     * RFC 7638 thumbprint of the device key
     *
     * @param string|null $jkt
     *
     * @return self
     */
    public function setJkt(?string $jkt): self
    {
        $this->initialized['jkt'] = true;
        $this->jkt = $jkt;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getLabel(): ?string
    {
        return $this->label;
    }
    /**
     * @param string|null $label
     *
     * @return self
     */
    public function setLabel(?string $label): self
    {
        $this->initialized['label'] = true;
        $this->label = $label;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getLastSeenAt(): ?\DateTime
    {
        return $this->lastSeenAt;
    }
    /**
     * @param \DateTime|null $lastSeenAt
     *
     * @return self
     */
    public function setLastSeenAt(?\DateTime $lastSeenAt): self
    {
        $this->initialized['lastSeenAt'] = true;
        $this->lastSeenAt = $lastSeenAt;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getMode(): ?string
    {
        return $this->mode;
    }
    /**
     * @param string|null $mode
     *
     * @return self
     */
    public function setMode(?string $mode): self
    {
        $this->initialized['mode'] = true;
        $this->mode = $mode;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getModel(): ?string
    {
        return $this->model;
    }
    /**
     * @param string|null $model
     *
     * @return self
     */
    public function setModel(?string $model): self
    {
        $this->initialized['model'] = true;
        $this->model = $model;
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
}