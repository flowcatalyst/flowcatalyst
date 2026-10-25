<?php

namespace FlowCatalyst\Generated\Model;

class IngestionSecretFetchResponse
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
     * How many times this runtime has fetched it
     *
     * @var int|null
     */
    protected $fetchCount;
    /**
     * @var \DateTime|null
     */
    protected $lastFetchedAt;
    /**
     * The runtime's service account that made it
     *
     * @var string|null
     */
    protected $principalId;
    /**
     * @var string|null
     */
    protected $runtimeId;
    /**
     * The live version the last fetch was made under
     *
     * @var int|null
     */
    protected $version;
    /**
     * How many times this runtime has fetched it
     *
     * @return int|null
     */
    public function getFetchCount(): ?int
    {
        return $this->fetchCount;
    }
    /**
     * How many times this runtime has fetched it
     *
     * @param int|null $fetchCount
     *
     * @return self
     */
    public function setFetchCount(?int $fetchCount): self
    {
        $this->initialized['fetchCount'] = true;
        $this->fetchCount = $fetchCount;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getLastFetchedAt(): ?\DateTime
    {
        return $this->lastFetchedAt;
    }
    /**
     * @param \DateTime|null $lastFetchedAt
     *
     * @return self
     */
    public function setLastFetchedAt(?\DateTime $lastFetchedAt): self
    {
        $this->initialized['lastFetchedAt'] = true;
        $this->lastFetchedAt = $lastFetchedAt;
        return $this;
    }
    /**
     * The runtime's service account that made it
     *
     * @return string|null
     */
    public function getPrincipalId(): ?string
    {
        return $this->principalId;
    }
    /**
     * The runtime's service account that made it
     *
     * @param string|null $principalId
     *
     * @return self
     */
    public function setPrincipalId(?string $principalId): self
    {
        $this->initialized['principalId'] = true;
        $this->principalId = $principalId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getRuntimeId(): ?string
    {
        return $this->runtimeId;
    }
    /**
     * @param string|null $runtimeId
     *
     * @return self
     */
    public function setRuntimeId(?string $runtimeId): self
    {
        $this->initialized['runtimeId'] = true;
        $this->runtimeId = $runtimeId;
        return $this;
    }
    /**
     * The live version the last fetch was made under
     *
     * @return int|null
     */
    public function getVersion(): ?int
    {
        return $this->version;
    }
    /**
     * The live version the last fetch was made under
     *
     * @param int|null $version
     *
     * @return self
     */
    public function setVersion(?int $version): self
    {
        $this->initialized['version'] = true;
        $this->version = $version;
        return $this;
    }
}