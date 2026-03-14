<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminLoginAttemptsGetResponse200ItemsItem extends \ArrayObject
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
    protected $id;
    /**
     * @var string|null
     */
    protected $attemptType;
    /**
     * @var string|null
     */
    protected $outcome;
    /**
     * @var mixed|null
     */
    protected $failureReason;
    /**
     * @var string|null
     */
    protected $identifier;
    /**
     * @var mixed|null
     */
    protected $principalId;
    /**
     * @var mixed|null
     */
    protected $ipAddress;
    /**
     * @var mixed|null
     */
    protected $userAgent;
    /**
     * @var \DateTime|null
     */
    protected $attemptedAt;
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
    public function getAttemptType(): ?string
    {
        return $this->attemptType;
    }
    /**
     * @param string|null $attemptType
     *
     * @return self
     */
    public function setAttemptType(?string $attemptType): self
    {
        $this->initialized['attemptType'] = true;
        $this->attemptType = $attemptType;
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
    /**
     * @return mixed
     */
    public function getFailureReason()
    {
        return $this->failureReason;
    }
    /**
     * @param mixed $failureReason
     *
     * @return self
     */
    public function setFailureReason($failureReason): self
    {
        $this->initialized['failureReason'] = true;
        $this->failureReason = $failureReason;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getIdentifier(): ?string
    {
        return $this->identifier;
    }
    /**
     * @param string|null $identifier
     *
     * @return self
     */
    public function setIdentifier(?string $identifier): self
    {
        $this->initialized['identifier'] = true;
        $this->identifier = $identifier;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getPrincipalId()
    {
        return $this->principalId;
    }
    /**
     * @param mixed $principalId
     *
     * @return self
     */
    public function setPrincipalId($principalId): self
    {
        $this->initialized['principalId'] = true;
        $this->principalId = $principalId;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getIpAddress()
    {
        return $this->ipAddress;
    }
    /**
     * @param mixed $ipAddress
     *
     * @return self
     */
    public function setIpAddress($ipAddress): self
    {
        $this->initialized['ipAddress'] = true;
        $this->ipAddress = $ipAddress;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getUserAgent()
    {
        return $this->userAgent;
    }
    /**
     * @param mixed $userAgent
     *
     * @return self
     */
    public function setUserAgent($userAgent): self
    {
        $this->initialized['userAgent'] = true;
        $this->userAgent = $userAgent;
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
}