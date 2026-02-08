<?php

namespace FlowCatalyst\Generated\Model;

class DispatchAttemptResponse extends \ArrayObject
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
     * @var int|null
     */
    protected $attemptNumber;
    /**
     * @var \DateTime|null
     */
    protected $attemptedAt;
    /**
     * @var \DateTime|null
     */
    protected $completedAt;
    /**
     * @var int|null
     */
    protected $durationMillis;
    /**
     * @var string|null
     */
    protected $status;
    /**
     * @var int|null
     */
    protected $responseCode;
    /**
     * @var string|null
     */
    protected $responseBody;
    /**
     * @var string|null
     */
    protected $errorMessage;
    /**
     * @var string|null
     */
    protected $errorType;
    /**
     * @var \DateTime|null
     */
    protected $createdAt;
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
     * @return int|null
     */
    public function getAttemptNumber(): ?int
    {
        return $this->attemptNumber;
    }
    /**
     * @param int|null $attemptNumber
     *
     * @return self
     */
    public function setAttemptNumber(?int $attemptNumber): self
    {
        $this->initialized['attemptNumber'] = true;
        $this->attemptNumber = $attemptNumber;
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
     * @return \DateTime|null
     */
    public function getCompletedAt(): ?\DateTime
    {
        return $this->completedAt;
    }
    /**
     * @param \DateTime|null $completedAt
     *
     * @return self
     */
    public function setCompletedAt(?\DateTime $completedAt): self
    {
        $this->initialized['completedAt'] = true;
        $this->completedAt = $completedAt;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getDurationMillis(): ?int
    {
        return $this->durationMillis;
    }
    /**
     * @param int|null $durationMillis
     *
     * @return self
     */
    public function setDurationMillis(?int $durationMillis): self
    {
        $this->initialized['durationMillis'] = true;
        $this->durationMillis = $durationMillis;
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
    public function getResponseCode(): ?int
    {
        return $this->responseCode;
    }
    /**
     * @param int|null $responseCode
     *
     * @return self
     */
    public function setResponseCode(?int $responseCode): self
    {
        $this->initialized['responseCode'] = true;
        $this->responseCode = $responseCode;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getResponseBody(): ?string
    {
        return $this->responseBody;
    }
    /**
     * @param string|null $responseBody
     *
     * @return self
     */
    public function setResponseBody(?string $responseBody): self
    {
        $this->initialized['responseBody'] = true;
        $this->responseBody = $responseBody;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getErrorMessage(): ?string
    {
        return $this->errorMessage;
    }
    /**
     * @param string|null $errorMessage
     *
     * @return self
     */
    public function setErrorMessage(?string $errorMessage): self
    {
        $this->initialized['errorMessage'] = true;
        $this->errorMessage = $errorMessage;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getErrorType(): ?string
    {
        return $this->errorType;
    }
    /**
     * @param string|null $errorType
     *
     * @return self
     */
    public function setErrorType(?string $errorType): self
    {
        $this->initialized['errorType'] = true;
        $this->errorType = $errorType;
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
}