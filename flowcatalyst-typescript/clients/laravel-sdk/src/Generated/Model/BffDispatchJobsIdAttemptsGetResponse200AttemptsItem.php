<?php

namespace FlowCatalyst\Generated\Model;

class BffDispatchJobsIdAttemptsGetResponse200AttemptsItem extends \ArrayObject
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
    protected $attemptNumber;
    /**
     * @var mixed|null
     */
    protected $status;
    /**
     * @var mixed|null
     */
    protected $responseCode;
    /**
     * @var mixed|null
     */
    protected $responseBody;
    /**
     * @var mixed|null
     */
    protected $errorMessage;
    /**
     * @var mixed|null
     */
    protected $errorType;
    /**
     * @var mixed|null
     */
    protected $durationMillis;
    /**
     * @var mixed|null
     */
    protected $attemptedAt;
    /**
     * @var mixed|null
     */
    protected $completedAt;
    /**
     * @var mixed|null
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
     * @return mixed
     */
    public function getAttemptNumber()
    {
        return $this->attemptNumber;
    }
    /**
     * @param mixed $attemptNumber
     *
     * @return self
     */
    public function setAttemptNumber($attemptNumber): self
    {
        $this->initialized['attemptNumber'] = true;
        $this->attemptNumber = $attemptNumber;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getStatus()
    {
        return $this->status;
    }
    /**
     * @param mixed $status
     *
     * @return self
     */
    public function setStatus($status): self
    {
        $this->initialized['status'] = true;
        $this->status = $status;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getResponseCode()
    {
        return $this->responseCode;
    }
    /**
     * @param mixed $responseCode
     *
     * @return self
     */
    public function setResponseCode($responseCode): self
    {
        $this->initialized['responseCode'] = true;
        $this->responseCode = $responseCode;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getResponseBody()
    {
        return $this->responseBody;
    }
    /**
     * @param mixed $responseBody
     *
     * @return self
     */
    public function setResponseBody($responseBody): self
    {
        $this->initialized['responseBody'] = true;
        $this->responseBody = $responseBody;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getErrorMessage()
    {
        return $this->errorMessage;
    }
    /**
     * @param mixed $errorMessage
     *
     * @return self
     */
    public function setErrorMessage($errorMessage): self
    {
        $this->initialized['errorMessage'] = true;
        $this->errorMessage = $errorMessage;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getErrorType()
    {
        return $this->errorType;
    }
    /**
     * @param mixed $errorType
     *
     * @return self
     */
    public function setErrorType($errorType): self
    {
        $this->initialized['errorType'] = true;
        $this->errorType = $errorType;
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
    public function getAttemptedAt()
    {
        return $this->attemptedAt;
    }
    /**
     * @param mixed $attemptedAt
     *
     * @return self
     */
    public function setAttemptedAt($attemptedAt): self
    {
        $this->initialized['attemptedAt'] = true;
        $this->attemptedAt = $attemptedAt;
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
    public function getCreatedAt()
    {
        return $this->createdAt;
    }
    /**
     * @param mixed $createdAt
     *
     * @return self
     */
    public function setCreatedAt($createdAt): self
    {
        $this->initialized['createdAt'] = true;
        $this->createdAt = $createdAt;
        return $this;
    }
}