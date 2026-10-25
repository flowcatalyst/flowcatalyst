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
}