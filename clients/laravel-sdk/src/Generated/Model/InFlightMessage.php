<?php

namespace FlowCatalyst\Generated\Model;

class InFlightMessage extends \ArrayObject
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
     * @var int|null
     */
    protected $attempt;
    /**
     * @var int|null
     */
    protected $elapsedMs;
    /**
     * @var string|null
     */
    protected $eventId;
    /**
     * @var string|null
     */
    protected $jobId;
    /**
     * @var string|null
     */
    protected $messageGroup;
    /**
     * @var string|null
     */
    protected $poolId;
    /**
     * @var string|null
     */
    protected $startedAt;
    /**
     * @var string|null
     */
    protected $targetUrl;
    /**
     * @return int|null
     */
    public function getAttempt(): ?int
    {
        return $this->attempt;
    }
    /**
     * @param int|null $attempt
     *
     * @return self
     */
    public function setAttempt(?int $attempt): self
    {
        $this->initialized['attempt'] = true;
        $this->attempt = $attempt;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getElapsedMs(): ?int
    {
        return $this->elapsedMs;
    }
    /**
     * @param int|null $elapsedMs
     *
     * @return self
     */
    public function setElapsedMs(?int $elapsedMs): self
    {
        $this->initialized['elapsedMs'] = true;
        $this->elapsedMs = $elapsedMs;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getEventId(): ?string
    {
        return $this->eventId;
    }
    /**
     * @param string|null $eventId
     *
     * @return self
     */
    public function setEventId(?string $eventId): self
    {
        $this->initialized['eventId'] = true;
        $this->eventId = $eventId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getJobId(): ?string
    {
        return $this->jobId;
    }
    /**
     * @param string|null $jobId
     *
     * @return self
     */
    public function setJobId(?string $jobId): self
    {
        $this->initialized['jobId'] = true;
        $this->jobId = $jobId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getMessageGroup(): ?string
    {
        return $this->messageGroup;
    }
    /**
     * @param string|null $messageGroup
     *
     * @return self
     */
    public function setMessageGroup(?string $messageGroup): self
    {
        $this->initialized['messageGroup'] = true;
        $this->messageGroup = $messageGroup;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getPoolId(): ?string
    {
        return $this->poolId;
    }
    /**
     * @param string|null $poolId
     *
     * @return self
     */
    public function setPoolId(?string $poolId): self
    {
        $this->initialized['poolId'] = true;
        $this->poolId = $poolId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getStartedAt(): ?string
    {
        return $this->startedAt;
    }
    /**
     * @param string|null $startedAt
     *
     * @return self
     */
    public function setStartedAt(?string $startedAt): self
    {
        $this->initialized['startedAt'] = true;
        $this->startedAt = $startedAt;
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
}