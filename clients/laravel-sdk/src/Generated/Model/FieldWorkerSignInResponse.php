<?php

namespace FlowCatalyst\Generated\Model;

class FieldWorkerSignInResponse
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
    protected $actorId;
    /**
     * @var string|null
     */
    protected $appCode;
    /**
     * @var \DateTime|null
     */
    protected $attemptedAt;
    /**
     * @var string|null
     */
    protected $authMethod;
    /**
     * @var string|null
     */
    protected $deviceId;
    /**
     * @var string|null
     */
    protected $failureReason;
    /**
     * @var string|null
     */
    protected $id;
    /**
     * @var string|null
     */
    protected $ipAddress;
    /**
     * @var string|null
     */
    protected $outcome;
    /**
     * @return string|null
     */
    public function getActorId(): ?string
    {
        return $this->actorId;
    }
    /**
     * @param string|null $actorId
     *
     * @return self
     */
    public function setActorId(?string $actorId): self
    {
        $this->initialized['actorId'] = true;
        $this->actorId = $actorId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getAppCode(): ?string
    {
        return $this->appCode;
    }
    /**
     * @param string|null $appCode
     *
     * @return self
     */
    public function setAppCode(?string $appCode): self
    {
        $this->initialized['appCode'] = true;
        $this->appCode = $appCode;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getAttemptedAt(): ?\DateTime
    {
        return $this->attemptedAt;
    }
    /**
     * @param \DateTime|null $attemptedAt
     *
     * @return self
     */
    public function setAttemptedAt(?\DateTime $attemptedAt): self
    {
        $this->initialized['attemptedAt'] = true;
        $this->attemptedAt = $attemptedAt;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getAuthMethod(): ?string
    {
        return $this->authMethod;
    }
    /**
     * @param string|null $authMethod
     *
     * @return self
     */
    public function setAuthMethod(?string $authMethod): self
    {
        $this->initialized['authMethod'] = true;
        $this->authMethod = $authMethod;
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
    public function getFailureReason(): ?string
    {
        return $this->failureReason;
    }
    /**
     * @param string|null $failureReason
     *
     * @return self
     */
    public function setFailureReason(?string $failureReason): self
    {
        $this->initialized['failureReason'] = true;
        $this->failureReason = $failureReason;
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
     * @return string|null
     */
    public function getIpAddress(): ?string
    {
        return $this->ipAddress;
    }
    /**
     * @param string|null $ipAddress
     *
     * @return self
     */
    public function setIpAddress(?string $ipAddress): self
    {
        $this->initialized['ipAddress'] = true;
        $this->ipAddress = $ipAddress;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getOutcome(): ?string
    {
        return $this->outcome;
    }
    /**
     * @param string|null $outcome
     *
     * @return self
     */
    public function setOutcome(?string $outcome): self
    {
        $this->initialized['outcome'] = true;
        $this->outcome = $outcome;
        return $this;
    }
}