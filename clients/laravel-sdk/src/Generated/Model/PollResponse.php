<?php

namespace FlowCatalyst\Generated\Model;

class PollResponse
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
     * When the platform read them
     *
     * @var \DateTime|null
     */
    protected $asOf;
    /**
     * Live versions only: every shared ingestion of a kind the runtime carries, and the polled ones whose lease it holds
     *
     * @var list<RuntimeIngestion>|null
     */
    protected $ingestions;
    /**
     * @var string|null
     */
    protected $leaseFor;
    /**
     * Pending test requests for kinds the runtime carries
     *
     * @var list<RuntimeTest>|null
     */
    protected $tests;
    /**
     * The open trace windows (hardStop in the future) of the ingestions listed above, for this runtime; absent when none. A windowId seen for the first time starts the runtime's own window of windowSecs, ending no later than hardStop; the same windowId again changes nothing; a different one replaces the window; an ingestion no longer listed has none.
     *
     * @var list<RuntimeTrace>|null
     */
    protected $traces;
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
     * When the platform read them
     *
     * @return \DateTime|null
     */
    public function getAsOf(): ?\DateTime
    {
        return $this->asOf;
    }
    /**
     * When the platform read them
     *
     * @param \DateTime|null $asOf
     *
     * @return self
     */
    public function setAsOf(?\DateTime $asOf): self
    {
        $this->initialized['asOf'] = true;
        $this->asOf = $asOf;
        return $this;
    }
    /**
     * Live versions only: every shared ingestion of a kind the runtime carries, and the polled ones whose lease it holds
     *
     * @return list<RuntimeIngestion>|null
     */
    public function getIngestions(): ?array
    {
        return $this->ingestions;
    }
    /**
     * Live versions only: every shared ingestion of a kind the runtime carries, and the polled ones whose lease it holds
     *
     * @param list<RuntimeIngestion>|null $ingestions
     *
     * @return self
     */
    public function setIngestions(?array $ingestions): self
    {
        $this->initialized['ingestions'] = true;
        $this->ingestions = $ingestions;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getLeaseFor(): ?string
    {
        return $this->leaseFor;
    }
    /**
     * @param string|null $leaseFor
     *
     * @return self
     */
    public function setLeaseFor(?string $leaseFor): self
    {
        $this->initialized['leaseFor'] = true;
        $this->leaseFor = $leaseFor;
        return $this;
    }
    /**
     * Pending test requests for kinds the runtime carries
     *
     * @return list<RuntimeTest>|null
     */
    public function getTests(): ?array
    {
        return $this->tests;
    }
    /**
     * Pending test requests for kinds the runtime carries
     *
     * @param list<RuntimeTest>|null $tests
     *
     * @return self
     */
    public function setTests(?array $tests): self
    {
        $this->initialized['tests'] = true;
        $this->tests = $tests;
        return $this;
    }
    /**
     * The open trace windows (hardStop in the future) of the ingestions listed above, for this runtime; absent when none. A windowId seen for the first time starts the runtime's own window of windowSecs, ending no later than hardStop; the same windowId again changes nothing; a different one replaces the window; an ingestion no longer listed has none.
     *
     * @return list<RuntimeTrace>|null
     */
    public function getTraces(): ?array
    {
        return $this->traces;
    }
    /**
     * The open trace windows (hardStop in the future) of the ingestions listed above, for this runtime; absent when none. A windowId seen for the first time starts the runtime's own window of windowSecs, ending no later than hardStop; the same windowId again changes nothing; a different one replaces the window; an ingestion no longer listed has none.
     *
     * @param list<RuntimeTrace>|null $traces
     *
     * @return self
     */
    public function setTraces(?array $traces): self
    {
        $this->initialized['traces'] = true;
        $this->traces = $traces;
        return $this;
    }
}