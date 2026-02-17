<?php

namespace FlowCatalyst\Generated\Model;

class ApiBffDispatchJobsIdGetResponse200 extends \ArrayObject
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
     * @var mixed|null
     */
    protected $externalId;
    /**
     * @var mixed|null
     */
    protected $source;
    /**
     * @var string|null
     */
    protected $kind;
    /**
     * @var string|null
     */
    protected $code;
    /**
     * @var mixed|null
     */
    protected $subject;
    /**
     * @var mixed|null
     */
    protected $eventId;
    /**
     * @var mixed|null
     */
    protected $correlationId;
    /**
     * @var string|null
     */
    protected $targetUrl;
    /**
     * @var string|null
     */
    protected $protocol;
    /**
     * @var mixed|null
     */
    protected $application;
    /**
     * @var mixed|null
     */
    protected $subdomain;
    /**
     * @var mixed|null
     */
    protected $aggregate;
    /**
     * @var mixed|null
     */
    protected $clientId;
    /**
     * @var mixed|null
     */
    protected $subscriptionId;
    /**
     * @var mixed|null
     */
    protected $serviceAccountId;
    /**
     * @var mixed|null
     */
    protected $dispatchPoolId;
    /**
     * @var mixed|null
     */
    protected $messageGroup;
    /**
     * @var string|null
     */
    protected $mode;
    /**
     * @var mixed|null
     */
    protected $sequence;
    /**
     * @var string|null
     */
    protected $status;
    /**
     * @var int|null
     */
    protected $attemptCount;
    /**
     * @var int|null
     */
    protected $maxRetries;
    /**
     * @var mixed|null
     */
    protected $lastError;
    /**
     * @var mixed|null
     */
    protected $timeoutSeconds;
    /**
     * @var mixed|null
     */
    protected $retryStrategy;
    /**
     * @var \DateTime|null
     */
    protected $createdAt;
    /**
     * @var \DateTime|null
     */
    protected $updatedAt;
    /**
     * @var mixed|null
     */
    protected $scheduledFor;
    /**
     * @var mixed|null
     */
    protected $expiresAt;
    /**
     * @var mixed|null
     */
    protected $completedAt;
    /**
     * @var mixed|null
     */
    protected $lastAttemptAt;
    /**
     * @var mixed|null
     */
    protected $durationMillis;
    /**
     * @var mixed|null
     */
    protected $idempotencyKey;
    /**
     * @var mixed|null
     */
    protected $isCompleted;
    /**
     * @var mixed|null
     */
    protected $isTerminal;
    /**
     * @var mixed|null
     */
    protected $projectedAt;
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
     * @return mixed
     */
    public function getExternalId()
    {
        return $this->externalId;
    }
    /**
     * @param mixed $externalId
     *
     * @return self
     */
    public function setExternalId($externalId): self
    {
        $this->initialized['externalId'] = true;
        $this->externalId = $externalId;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getSource()
    {
        return $this->source;
    }
    /**
     * @param mixed $source
     *
     * @return self
     */
    public function setSource($source): self
    {
        $this->initialized['source'] = true;
        $this->source = $source;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getKind(): ?string
    {
        return $this->kind;
    }
    /**
     * @param string|null $kind
     *
     * @return self
     */
    public function setKind(?string $kind): self
    {
        $this->initialized['kind'] = true;
        $this->kind = $kind;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getCode(): ?string
    {
        return $this->code;
    }
    /**
     * @param string|null $code
     *
     * @return self
     */
    public function setCode(?string $code): self
    {
        $this->initialized['code'] = true;
        $this->code = $code;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getSubject()
    {
        return $this->subject;
    }
    /**
     * @param mixed $subject
     *
     * @return self
     */
    public function setSubject($subject): self
    {
        $this->initialized['subject'] = true;
        $this->subject = $subject;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getEventId()
    {
        return $this->eventId;
    }
    /**
     * @param mixed $eventId
     *
     * @return self
     */
    public function setEventId($eventId): self
    {
        $this->initialized['eventId'] = true;
        $this->eventId = $eventId;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getCorrelationId()
    {
        return $this->correlationId;
    }
    /**
     * @param mixed $correlationId
     *
     * @return self
     */
    public function setCorrelationId($correlationId): self
    {
        $this->initialized['correlationId'] = true;
        $this->correlationId = $correlationId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getTargetUrl(): ?string
    {
        return $this->targetUrl;
    }
    /**
     * @param string|null $targetUrl
     *
     * @return self
     */
    public function setTargetUrl(?string $targetUrl): self
    {
        $this->initialized['targetUrl'] = true;
        $this->targetUrl = $targetUrl;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getProtocol(): ?string
    {
        return $this->protocol;
    }
    /**
     * @param string|null $protocol
     *
     * @return self
     */
    public function setProtocol(?string $protocol): self
    {
        $this->initialized['protocol'] = true;
        $this->protocol = $protocol;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getApplication()
    {
        return $this->application;
    }
    /**
     * @param mixed $application
     *
     * @return self
     */
    public function setApplication($application): self
    {
        $this->initialized['application'] = true;
        $this->application = $application;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getSubdomain()
    {
        return $this->subdomain;
    }
    /**
     * @param mixed $subdomain
     *
     * @return self
     */
    public function setSubdomain($subdomain): self
    {
        $this->initialized['subdomain'] = true;
        $this->subdomain = $subdomain;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getAggregate()
    {
        return $this->aggregate;
    }
    /**
     * @param mixed $aggregate
     *
     * @return self
     */
    public function setAggregate($aggregate): self
    {
        $this->initialized['aggregate'] = true;
        $this->aggregate = $aggregate;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getClientId()
    {
        return $this->clientId;
    }
    /**
     * @param mixed $clientId
     *
     * @return self
     */
    public function setClientId($clientId): self
    {
        $this->initialized['clientId'] = true;
        $this->clientId = $clientId;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getSubscriptionId()
    {
        return $this->subscriptionId;
    }
    /**
     * @param mixed $subscriptionId
     *
     * @return self
     */
    public function setSubscriptionId($subscriptionId): self
    {
        $this->initialized['subscriptionId'] = true;
        $this->subscriptionId = $subscriptionId;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getServiceAccountId()
    {
        return $this->serviceAccountId;
    }
    /**
     * @param mixed $serviceAccountId
     *
     * @return self
     */
    public function setServiceAccountId($serviceAccountId): self
    {
        $this->initialized['serviceAccountId'] = true;
        $this->serviceAccountId = $serviceAccountId;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getDispatchPoolId()
    {
        return $this->dispatchPoolId;
    }
    /**
     * @param mixed $dispatchPoolId
     *
     * @return self
     */
    public function setDispatchPoolId($dispatchPoolId): self
    {
        $this->initialized['dispatchPoolId'] = true;
        $this->dispatchPoolId = $dispatchPoolId;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getMessageGroup()
    {
        return $this->messageGroup;
    }
    /**
     * @param mixed $messageGroup
     *
     * @return self
     */
    public function setMessageGroup($messageGroup): self
    {
        $this->initialized['messageGroup'] = true;
        $this->messageGroup = $messageGroup;
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
     * @return mixed
     */
    public function getSequence()
    {
        return $this->sequence;
    }
    /**
     * @param mixed $sequence
     *
     * @return self
     */
    public function setSequence($sequence): self
    {
        $this->initialized['sequence'] = true;
        $this->sequence = $sequence;
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
     * @return int|null
     */
    public function getAttemptCount(): ?int
    {
        return $this->attemptCount;
    }
    /**
     * @param int|null $attemptCount
     *
     * @return self
     */
    public function setAttemptCount(?int $attemptCount): self
    {
        $this->initialized['attemptCount'] = true;
        $this->attemptCount = $attemptCount;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getMaxRetries(): ?int
    {
        return $this->maxRetries;
    }
    /**
     * @param int|null $maxRetries
     *
     * @return self
     */
    public function setMaxRetries(?int $maxRetries): self
    {
        $this->initialized['maxRetries'] = true;
        $this->maxRetries = $maxRetries;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getLastError()
    {
        return $this->lastError;
    }
    /**
     * @param mixed $lastError
     *
     * @return self
     */
    public function setLastError($lastError): self
    {
        $this->initialized['lastError'] = true;
        $this->lastError = $lastError;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getTimeoutSeconds()
    {
        return $this->timeoutSeconds;
    }
    /**
     * @param mixed $timeoutSeconds
     *
     * @return self
     */
    public function setTimeoutSeconds($timeoutSeconds): self
    {
        $this->initialized['timeoutSeconds'] = true;
        $this->timeoutSeconds = $timeoutSeconds;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getRetryStrategy()
    {
        return $this->retryStrategy;
    }
    /**
     * @param mixed $retryStrategy
     *
     * @return self
     */
    public function setRetryStrategy($retryStrategy): self
    {
        $this->initialized['retryStrategy'] = true;
        $this->retryStrategy = $retryStrategy;
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
     * @return mixed
     */
    public function getScheduledFor()
    {
        return $this->scheduledFor;
    }
    /**
     * @param mixed $scheduledFor
     *
     * @return self
     */
    public function setScheduledFor($scheduledFor): self
    {
        $this->initialized['scheduledFor'] = true;
        $this->scheduledFor = $scheduledFor;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getExpiresAt()
    {
        return $this->expiresAt;
    }
    /**
     * @param mixed $expiresAt
     *
     * @return self
     */
    public function setExpiresAt($expiresAt): self
    {
        $this->initialized['expiresAt'] = true;
        $this->expiresAt = $expiresAt;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getCompletedAt()
    {
        return $this->completedAt;
    }
    /**
     * @param mixed $completedAt
     *
     * @return self
     */
    public function setCompletedAt($completedAt): self
    {
        $this->initialized['completedAt'] = true;
        $this->completedAt = $completedAt;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getLastAttemptAt()
    {
        return $this->lastAttemptAt;
    }
    /**
     * @param mixed $lastAttemptAt
     *
     * @return self
     */
    public function setLastAttemptAt($lastAttemptAt): self
    {
        $this->initialized['lastAttemptAt'] = true;
        $this->lastAttemptAt = $lastAttemptAt;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getDurationMillis()
    {
        return $this->durationMillis;
    }
    /**
     * @param mixed $durationMillis
     *
     * @return self
     */
    public function setDurationMillis($durationMillis): self
    {
        $this->initialized['durationMillis'] = true;
        $this->durationMillis = $durationMillis;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getIdempotencyKey()
    {
        return $this->idempotencyKey;
    }
    /**
     * @param mixed $idempotencyKey
     *
     * @return self
     */
    public function setIdempotencyKey($idempotencyKey): self
    {
        $this->initialized['idempotencyKey'] = true;
        $this->idempotencyKey = $idempotencyKey;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getIsCompleted()
    {
        return $this->isCompleted;
    }
    /**
     * @param mixed $isCompleted
     *
     * @return self
     */
    public function setIsCompleted($isCompleted): self
    {
        $this->initialized['isCompleted'] = true;
        $this->isCompleted = $isCompleted;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getIsTerminal()
    {
        return $this->isTerminal;
    }
    /**
     * @param mixed $isTerminal
     *
     * @return self
     */
    public function setIsTerminal($isTerminal): self
    {
        $this->initialized['isTerminal'] = true;
        $this->isTerminal = $isTerminal;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getProjectedAt()
    {
        return $this->projectedAt;
    }
    /**
     * @param mixed $projectedAt
     *
     * @return self
     */
    public function setProjectedAt($projectedAt): self
    {
        $this->initialized['projectedAt'] = true;
        $this->projectedAt = $projectedAt;
        return $this;
    }
}