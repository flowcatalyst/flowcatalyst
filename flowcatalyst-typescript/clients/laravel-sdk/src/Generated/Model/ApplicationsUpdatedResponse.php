<?php

namespace FlowCatalyst\Generated\Model;

class ApplicationsUpdatedResponse extends \ArrayObject
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
     * Client ID
     *
     * @var string|null
     */
    protected $clientId;
    /**
     * Number of applications now enabled
     *
     * @var int|null
     */
    protected $enabledCount;
    /**
     * Human-readable message
     *
     * @var string|null
     */
    protected $message;
    /**
     * Client ID
     *
     * @return string|null
     */
    public function getClientId(): ?string
    {
        return $this->clientId;
    }
    /**
     * Client ID
     *
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
     * Number of applications now enabled
     *
     * @return int|null
     */
    public function getEnabledCount(): ?int
    {
        return $this->enabledCount;
    }
    /**
     * Number of applications now enabled
     *
     * @param int|null $enabledCount
     *
     * @return self
     */
    public function setEnabledCount(?int $enabledCount): self
    {
        $this->initialized['enabledCount'] = true;
        $this->enabledCount = $enabledCount;
        return $this;
    }
    /**
     * Human-readable message
     *
     * @return string|null
     */
    public function getMessage(): ?string
    {
        return $this->message;
    }
    /**
     * Human-readable message
     *
     * @param string|null $message
     *
     * @return self
     */
    public function setMessage(?string $message): self
    {
        $this->initialized['message'] = true;
        $this->message = $message;
        return $this;
    }
}