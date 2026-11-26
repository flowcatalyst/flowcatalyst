<?php

namespace FlowCatalyst\Generated\Model;

class ItemResponse
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
    protected $attempts;
    /**
     * client-id, client-secret, auth-token or signing-secret
     *
     * @var string|null
     */
    protected $credential;
    /**
     * @var string|null
     */
    protected $lastError;
    /**
     * @var \DateTime|null
     */
    protected $nextAttemptAt;
    /**
     * The full parameter name
     *
     * @var string|null
     */
    protected $path;
    /**
     * @var \DateTime|null
     */
    protected $publishedAt;
    /**
     * The secret reference a function or ingestion uses to read it back
     *
     * @var string|null
     */
    protected $reference;
    /**
     * @var string|null
     */
    protected $state;
    /**
     * @return int|null
     */
    public function getAttempts(): ?int
    {
        return $this->attempts;
    }
    /**
     * @param int|null $attempts
     *
     * @return self
     */
    public function setAttempts(?int $attempts): self
    {
        $this->initialized['attempts'] = true;
        $this->attempts = $attempts;
        return $this;
    }
    /**
     * client-id, client-secret, auth-token or signing-secret
     *
     * @return string|null
     */
    public function getCredential(): ?string
    {
        return $this->credential;
    }
    /**
     * client-id, client-secret, auth-token or signing-secret
     *
     * @param string|null $credential
     *
     * @return self
     */
    public function setCredential(?string $credential): self
    {
        $this->initialized['credential'] = true;
        $this->credential = $credential;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getLastError(): ?string
    {
        return $this->lastError;
    }
    /**
     * @param string|null $lastError
     *
     * @return self
     */
    public function setLastError(?string $lastError): self
    {
        $this->initialized['lastError'] = true;
        $this->lastError = $lastError;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getNextAttemptAt(): ?\DateTime
    {
        return $this->nextAttemptAt;
    }
    /**
     * @param \DateTime|null $nextAttemptAt
     *
     * @return self
     */
    public function setNextAttemptAt(?\DateTime $nextAttemptAt): self
    {
        $this->initialized['nextAttemptAt'] = true;
        $this->nextAttemptAt = $nextAttemptAt;
        return $this;
    }
    /**
     * The full parameter name
     *
     * @return string|null
     */
    public function getPath(): ?string
    {
        return $this->path;
    }
    /**
     * The full parameter name
     *
     * @param string|null $path
     *
     * @return self
     */
    public function setPath(?string $path): self
    {
        $this->initialized['path'] = true;
        $this->path = $path;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getPublishedAt(): ?\DateTime
    {
        return $this->publishedAt;
    }
    /**
     * @param \DateTime|null $publishedAt
     *
     * @return self
     */
    public function setPublishedAt(?\DateTime $publishedAt): self
    {
        $this->initialized['publishedAt'] = true;
        $this->publishedAt = $publishedAt;
        return $this;
    }
    /**
     * The secret reference a function or ingestion uses to read it back
     *
     * @return string|null
     */
    public function getReference(): ?string
    {
        return $this->reference;
    }
    /**
     * The secret reference a function or ingestion uses to read it back
     *
     * @param string|null $reference
     *
     * @return self
     */
    public function setReference(?string $reference): self
    {
        $this->initialized['reference'] = true;
        $this->reference = $reference;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getState(): ?string
    {
        return $this->state;
    }
    /**
     * @param string|null $state
     *
     * @return self
     */
    public function setState(?string $state): self
    {
        $this->initialized['state'] = true;
        $this->state = $state;
        return $this;
    }
}