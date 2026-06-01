<?php

namespace FlowCatalyst\Generated\Model;

class CreateSubscriptionRequest extends \ArrayObject
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
     * Client ID (optional, null = anchor-level)
     *
     * @var string|null
     */
    protected $clientId;
    /**
     * Unique code
     *
     * @var string|null
     */
    protected $code;
    /**
     * Connection ID (references msg_connections, optional)
     *
     * @var string|null
     */
    protected $connectionId;
    /**
     * Send raw event data only
     *
     * @var bool|null
     */
    protected $dataOnly;
    /**
     * Description
     *
     * @var string|null
     */
    protected $description;
    /**
     * Dispatch pool ID for rate limiting
     *
     * @var string|null
     */
    protected $dispatchPoolId;
    /**
     * Webhook endpoint URL
     *
     * @var string|null
     */
    protected $endpoint;
    /**
     * Event types to listen to
     *
     * @var list<EventTypeBindingRequest>|null
     */
    protected $eventTypes;
    /**
     * Maximum retry attempts
     *
     * @var int|null
     */
    protected $maxRetries;
    /**
     * Dispatch mode
     *
     * @var string|null
     */
    protected $mode;
    /**
     * Human-readable name
     *
     * @var string|null
     */
    protected $name;
    /**
     * Service account ID for authentication
     *
     * @var string|null
     */
    protected $serviceAccountId;
    /**
     * Timeout in seconds
     *
     * @var int|null
     */
    protected $timeoutSeconds;
    /**
     * Client ID (optional, null = anchor-level)
     *
     * @return string|null
     */
    public function getClientId(): ?string
    {
        return $this->clientId;
    }
    /**
     * Client ID (optional, null = anchor-level)
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
     * Unique code
     *
     * @return string|null
     */
    public function getCode(): ?string
    {
        return $this->code;
    }
    /**
     * Unique code
     *
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
     * Connection ID (references msg_connections, optional)
     *
     * @return string|null
     */
    public function getConnectionId(): ?string
    {
        return $this->connectionId;
    }
    /**
     * Connection ID (references msg_connections, optional)
     *
     * @param string|null $connectionId
     *
     * @return self
     */
    public function setConnectionId(?string $connectionId): self
    {
        $this->initialized['connectionId'] = true;
        $this->connectionId = $connectionId;
        return $this;
    }
    /**
     * Send raw event data only
     *
     * @return bool|null
     */
    public function getDataOnly(): ?bool
    {
        return $this->dataOnly;
    }
    /**
     * Send raw event data only
     *
     * @param bool|null $dataOnly
     *
     * @return self
     */
    public function setDataOnly(?bool $dataOnly): self
    {
        $this->initialized['dataOnly'] = true;
        $this->dataOnly = $dataOnly;
        return $this;
    }
    /**
     * Description
     *
     * @return string|null
     */
    public function getDescription(): ?string
    {
        return $this->description;
    }
    /**
     * Description
     *
     * @param string|null $description
     *
     * @return self
     */
    public function setDescription(?string $description): self
    {
        $this->initialized['description'] = true;
        $this->description = $description;
        return $this;
    }
    /**
     * Dispatch pool ID for rate limiting
     *
     * @return string|null
     */
    public function getDispatchPoolId(): ?string
    {
        return $this->dispatchPoolId;
    }
    /**
     * Dispatch pool ID for rate limiting
     *
     * @param string|null $dispatchPoolId
     *
     * @return self
     */
    public function setDispatchPoolId(?string $dispatchPoolId): self
    {
        $this->initialized['dispatchPoolId'] = true;
        $this->dispatchPoolId = $dispatchPoolId;
        return $this;
    }
    /**
     * Webhook endpoint URL
     *
     * @return string|null
     */
    public function getEndpoint(): ?string
    {
        return $this->endpoint;
    }
    /**
     * Webhook endpoint URL
     *
     * @param string|null $endpoint
     *
     * @return self
     */
    public function setEndpoint(?string $endpoint): self
    {
        $this->initialized['endpoint'] = true;
        $this->endpoint = $endpoint;
        return $this;
    }
    /**
     * Event types to listen to
     *
     * @return list<EventTypeBindingRequest>|null
     */
    public function getEventTypes(): ?array
    {
        return $this->eventTypes;
    }
    /**
     * Event types to listen to
     *
     * @param list<EventTypeBindingRequest>|null $eventTypes
     *
     * @return self
     */
    public function setEventTypes(?array $eventTypes): self
    {
        $this->initialized['eventTypes'] = true;
        $this->eventTypes = $eventTypes;
        return $this;
    }
    /**
     * Maximum retry attempts
     *
     * @return int|null
     */
    public function getMaxRetries(): ?int
    {
        return $this->maxRetries;
    }
    /**
     * Maximum retry attempts
     *
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
     * Dispatch mode
     *
     * @return string|null
     */
    public function getMode(): ?string
    {
        return $this->mode;
    }
    /**
     * Dispatch mode
     *
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
     * Human-readable name
     *
     * @return string|null
     */
    public function getName(): ?string
    {
        return $this->name;
    }
    /**
     * Human-readable name
     *
     * @param string|null $name
     *
     * @return self
     */
    public function setName(?string $name): self
    {
        $this->initialized['name'] = true;
        $this->name = $name;
        return $this;
    }
    /**
     * Service account ID for authentication
     *
     * @return string|null
     */
    public function getServiceAccountId(): ?string
    {
        return $this->serviceAccountId;
    }
    /**
     * Service account ID for authentication
     *
     * @param string|null $serviceAccountId
     *
     * @return self
     */
    public function setServiceAccountId(?string $serviceAccountId): self
    {
        $this->initialized['serviceAccountId'] = true;
        $this->serviceAccountId = $serviceAccountId;
        return $this;
    }
    /**
     * Timeout in seconds
     *
     * @return int|null
     */
    public function getTimeoutSeconds(): ?int
    {
        return $this->timeoutSeconds;
    }
    /**
     * Timeout in seconds
     *
     * @param int|null $timeoutSeconds
     *
     * @return self
     */
    public function setTimeoutSeconds(?int $timeoutSeconds): self
    {
        $this->initialized['timeoutSeconds'] = true;
        $this->timeoutSeconds = $timeoutSeconds;
        return $this;
    }
}