<?php

namespace FlowCatalyst\Generated\Model;

class RequeueByFilterRequest extends \ArrayObject
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
     * The eligibleCount the caller was shown. If more jobs are eligible now, nothing is reset and the response is 409 FILTER_MATCH_CHANGED.
     *
     * @var int|null
     */
    protected $confirmCount;
    /**
     * @var string|null
     */
    protected $dispatchPoolId;
    /**
     * Most jobs to reset in this request (default and maximum 10000)
     *
     * @var int|null
     */
    protected $limit;
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
     * @var string|null
     */
    protected $subscriptionId;
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
     * The eligibleCount the caller was shown. If more jobs are eligible now, nothing is reset and the response is 409 FILTER_MATCH_CHANGED.
     *
     * @return int|null
     */
    public function getConfirmCount(): ?int
    {
        return $this->confirmCount;
    }
    /**
     * The eligibleCount the caller was shown. If more jobs are eligible now, nothing is reset and the response is 409 FILTER_MATCH_CHANGED.
     *
     * @param int|null $confirmCount
     *
     * @return self
     */
    public function setConfirmCount(?int $confirmCount): self
    {
        $this->initialized['confirmCount'] = true;
        $this->confirmCount = $confirmCount;
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
     * Most jobs to reset in this request (default and maximum 10000)
     *
     * @return int|null
     */
    public function getLimit(): ?int
    {
        return $this->limit;
    }
    /**
     * Most jobs to reset in this request (default and maximum 10000)
     *
     * @param int|null $limit
     *
     * @return self
     */
    public function setLimit(?int $limit): self
    {
        $this->initialized['limit'] = true;
        $this->limit = $limit;
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