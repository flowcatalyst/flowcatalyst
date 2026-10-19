<?php

namespace FlowCatalyst\Generated\Model;

class RequeueByFilterPreviewRequest extends \ArrayObject
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
     * CSV of aggregates
     *
     * @var string|null
     */
    protected $aggregates;
    /**
     * CSV of application codes
     *
     * @var string|null
     */
    protected $applications;
    /**
     * @var string|null
     */
    protected $clientId;
    /**
     * CSV of client ids
     *
     * @var string|null
     */
    protected $clientIds;
    /**
     * @var string|null
     */
    protected $code;
    /**
     * CSV of codes
     *
     * @var string|null
     */
    protected $codes;
    /**
     * Connection code, exact; a trailing * makes it a prefix. Resolved to connection ids first (400 TOO_MANY_MATCHES past 50); a code that matches nothing lists no jobs.
     *
     * @var string|null
     */
    protected $connectionCode;
    /**
     * Jobs delivered through this connection: the jobs of its subscriptions and the jobs that name the connection themselves (function dispatch).
     *
     * @var string|null
     */
    protected $connectionId;
    /**
     * @var string|null
     */
    protected $dispatchPoolId;
    /**
     * Exact event type. A job's code is its event type for a job raised by an event (and the caller's own job code for an SDK or function-dispatched job), so this filters the same column as code, by an index that also orders by createdAt. Combines with eventTypes and codes (all must hold).
     *
     * @var string|null
     */
    protected $eventType;
    /**
     * CSV of event types (at most 10; more is 400 TOO_MANY_EVENT_TYPES)
     *
     * @var string|null
     */
    protected $eventTypes;
    /**
     * Exact message group
     *
     * @var string|null
     */
    protected $messageGroup;
    /**
     * RFC3339 timestamp
     *
     * @var string|null
     */
    protected $since;
    /**
     * Free-text source filter
     *
     * @var string|null
     */
    protected $source;
    /**
     * @var string|null
     */
    protected $status;
    /**
     * CSV of statuses
     *
     * @var string|null
     */
    protected $statuses;
    /**
     * CSV of subdomains
     *
     * @var string|null
     */
    protected $subdomains;
    /**
     * Subscription code, exact; a trailing * makes it a prefix. Resolved to subscription ids first (400 TOO_MANY_MATCHES past 200); a code that matches nothing lists no jobs.
     *
     * @var string|null
     */
    protected $subscriptionCode;
    /**
     * @var string|null
     */
    protected $subscriptionId;
    /**
     * CSV of subscription ids (at most 50; more is 400 TOO_MANY_SUBSCRIPTION_IDS). Combines with subscriptionId.
     *
     * @var string|null
     */
    protected $subscriptionIds;
    /**
     * Subscription name, exact and case-insensitive; a trailing * makes it a prefix. Resolved like subscriptionCode.
     *
     * @var string|null
     */
    protected $subscriptionName;
    /**
     * RFC3339 timestamp
     *
     * @var string|null
     */
    protected $until;
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
     * CSV of aggregates
     *
     * @return string|null
     */
    public function getAggregates(): ?string
    {
        return $this->aggregates;
    }
    /**
     * CSV of aggregates
     *
     * @param string|null $aggregates
     *
     * @return self
     */
    public function setAggregates(?string $aggregates): self
    {
        $this->initialized['aggregates'] = true;
        $this->aggregates = $aggregates;
        return $this;
    }
    /**
     * CSV of application codes
     *
     * @return string|null
     */
    public function getApplications(): ?string
    {
        return $this->applications;
    }
    /**
     * CSV of application codes
     *
     * @param string|null $applications
     *
     * @return self
     */
    public function setApplications(?string $applications): self
    {
        $this->initialized['applications'] = true;
        $this->applications = $applications;
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
     * CSV of client ids
     *
     * @return string|null
     */
    public function getClientIds(): ?string
    {
        return $this->clientIds;
    }
    /**
     * CSV of client ids
     *
     * @param string|null $clientIds
     *
     * @return self
     */
    public function setClientIds(?string $clientIds): self
    {
        $this->initialized['clientIds'] = true;
        $this->clientIds = $clientIds;
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
     * CSV of codes
     *
     * @return string|null
     */
    public function getCodes(): ?string
    {
        return $this->codes;
    }
    /**
     * CSV of codes
     *
     * @param string|null $codes
     *
     * @return self
     */
    public function setCodes(?string $codes): self
    {
        $this->initialized['codes'] = true;
        $this->codes = $codes;
        return $this;
    }
    /**
     * Connection code, exact; a trailing * makes it a prefix. Resolved to connection ids first (400 TOO_MANY_MATCHES past 50); a code that matches nothing lists no jobs.
     *
     * @return string|null
     */
    public function getConnectionCode(): ?string
    {
        return $this->connectionCode;
    }
    /**
     * Connection code, exact; a trailing * makes it a prefix. Resolved to connection ids first (400 TOO_MANY_MATCHES past 50); a code that matches nothing lists no jobs.
     *
     * @param string|null $connectionCode
     *
     * @return self
     */
    public function setConnectionCode(?string $connectionCode): self
    {
        $this->initialized['connectionCode'] = true;
        $this->connectionCode = $connectionCode;
        return $this;
    }
    /**
     * Jobs delivered through this connection: the jobs of its subscriptions and the jobs that name the connection themselves (function dispatch).
     *
     * @return string|null
     */
    public function getConnectionId(): ?string
    {
        return $this->connectionId;
    }
    /**
     * Jobs delivered through this connection: the jobs of its subscriptions and the jobs that name the connection themselves (function dispatch).
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
     * @return string|null
     */
    public function getDispatchPoolId(): ?string
    {
        return $this->dispatchPoolId;
    }
    /**
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
     * Exact event type. A job's code is its event type for a job raised by an event (and the caller's own job code for an SDK or function-dispatched job), so this filters the same column as code, by an index that also orders by createdAt. Combines with eventTypes and codes (all must hold).
     *
     * @return string|null
     */
    public function getEventType(): ?string
    {
        return $this->eventType;
    }
    /**
     * Exact event type. A job's code is its event type for a job raised by an event (and the caller's own job code for an SDK or function-dispatched job), so this filters the same column as code, by an index that also orders by createdAt. Combines with eventTypes and codes (all must hold).
     *
     * @param string|null $eventType
     *
     * @return self
     */
    public function setEventType(?string $eventType): self
    {
        $this->initialized['eventType'] = true;
        $this->eventType = $eventType;
        return $this;
    }
    /**
     * CSV of event types (at most 10; more is 400 TOO_MANY_EVENT_TYPES)
     *
     * @return string|null
     */
    public function getEventTypes(): ?string
    {
        return $this->eventTypes;
    }
    /**
     * CSV of event types (at most 10; more is 400 TOO_MANY_EVENT_TYPES)
     *
     * @param string|null $eventTypes
     *
     * @return self
     */
    public function setEventTypes(?string $eventTypes): self
    {
        $this->initialized['eventTypes'] = true;
        $this->eventTypes = $eventTypes;
        return $this;
    }
    /**
     * Exact message group
     *
     * @return string|null
     */
    public function getMessageGroup(): ?string
    {
        return $this->messageGroup;
    }
    /**
     * Exact message group
     *
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
     * RFC3339 timestamp
     *
     * @return string|null
     */
    public function getSince(): ?string
    {
        return $this->since;
    }
    /**
     * RFC3339 timestamp
     *
     * @param string|null $since
     *
     * @return self
     */
    public function setSince(?string $since): self
    {
        $this->initialized['since'] = true;
        $this->since = $since;
        return $this;
    }
    /**
     * Free-text source filter
     *
     * @return string|null
     */
    public function getSource(): ?string
    {
        return $this->source;
    }
    /**
     * Free-text source filter
     *
     * @param string|null $source
     *
     * @return self
     */
    public function setSource(?string $source): self
    {
        $this->initialized['source'] = true;
        $this->source = $source;
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
     * CSV of statuses
     *
     * @return string|null
     */
    public function getStatuses(): ?string
    {
        return $this->statuses;
    }
    /**
     * CSV of statuses
     *
     * @param string|null $statuses
     *
     * @return self
     */
    public function setStatuses(?string $statuses): self
    {
        $this->initialized['statuses'] = true;
        $this->statuses = $statuses;
        return $this;
    }
    /**
     * CSV of subdomains
     *
     * @return string|null
     */
    public function getSubdomains(): ?string
    {
        return $this->subdomains;
    }
    /**
     * CSV of subdomains
     *
     * @param string|null $subdomains
     *
     * @return self
     */
    public function setSubdomains(?string $subdomains): self
    {
        $this->initialized['subdomains'] = true;
        $this->subdomains = $subdomains;
        return $this;
    }
    /**
     * Subscription code, exact; a trailing * makes it a prefix. Resolved to subscription ids first (400 TOO_MANY_MATCHES past 200); a code that matches nothing lists no jobs.
     *
     * @return string|null
     */
    public function getSubscriptionCode(): ?string
    {
        return $this->subscriptionCode;
    }
    /**
     * Subscription code, exact; a trailing * makes it a prefix. Resolved to subscription ids first (400 TOO_MANY_MATCHES past 200); a code that matches nothing lists no jobs.
     *
     * @param string|null $subscriptionCode
     *
     * @return self
     */
    public function setSubscriptionCode(?string $subscriptionCode): self
    {
        $this->initialized['subscriptionCode'] = true;
        $this->subscriptionCode = $subscriptionCode;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getSubscriptionId(): ?string
    {
        return $this->subscriptionId;
    }
    /**
     * @param string|null $subscriptionId
     *
     * @return self
     */
    public function setSubscriptionId(?string $subscriptionId): self
    {
        $this->initialized['subscriptionId'] = true;
        $this->subscriptionId = $subscriptionId;
        return $this;
    }
    /**
     * CSV of subscription ids (at most 50; more is 400 TOO_MANY_SUBSCRIPTION_IDS). Combines with subscriptionId.
     *
     * @return string|null
     */
    public function getSubscriptionIds(): ?string
    {
        return $this->subscriptionIds;
    }
    /**
     * CSV of subscription ids (at most 50; more is 400 TOO_MANY_SUBSCRIPTION_IDS). Combines with subscriptionId.
     *
     * @param string|null $subscriptionIds
     *
     * @return self
     */
    public function setSubscriptionIds(?string $subscriptionIds): self
    {
        $this->initialized['subscriptionIds'] = true;
        $this->subscriptionIds = $subscriptionIds;
        return $this;
    }
    /**
     * Subscription name, exact and case-insensitive; a trailing * makes it a prefix. Resolved like subscriptionCode.
     *
     * @return string|null
     */
    public function getSubscriptionName(): ?string
    {
        return $this->subscriptionName;
    }
    /**
     * Subscription name, exact and case-insensitive; a trailing * makes it a prefix. Resolved like subscriptionCode.
     *
     * @param string|null $subscriptionName
     *
     * @return self
     */
    public function setSubscriptionName(?string $subscriptionName): self
    {
        $this->initialized['subscriptionName'] = true;
        $this->subscriptionName = $subscriptionName;
        return $this;
    }
    /**
     * RFC3339 timestamp
     *
     * @return string|null
     */
    public function getUntil(): ?string
    {
        return $this->until;
    }
    /**
     * RFC3339 timestamp
     *
     * @param string|null $until
     *
     * @return self
     */
    public function setUntil(?string $until): self
    {
        $this->initialized['until'] = true;
        $this->until = $until;
        return $this;
    }
}