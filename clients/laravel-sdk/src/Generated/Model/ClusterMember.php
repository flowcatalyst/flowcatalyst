<?php

namespace FlowCatalyst\Generated\Model;

class ClusterMember extends \ArrayObject
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
     * @var bool|null
     */
    protected $healthy;
    /**
     * @var string|null
     */
    protected $instanceId;
    /**
     * @var string|null
     */
    protected $lastSeen;
    /**
     * @var string|null
     */
    protected $role;
    /**
     * @return bool|null
     */
    public function getHealthy(): ?bool
    {
        return $this->healthy;
    }
    /**
     * @param bool|null $healthy
     *
     * @return self
     */
    public function setHealthy(?bool $healthy): self
    {
        $this->initialized['healthy'] = true;
        $this->healthy = $healthy;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getInstanceId(): ?string
    {
        return $this->instanceId;
    }
    /**
     * @param string|null $instanceId
     *
     * @return self
     */
    public function setInstanceId(?string $instanceId): self
    {
        $this->initialized['instanceId'] = true;
        $this->instanceId = $instanceId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getLastSeen(): ?string
    {
        return $this->lastSeen;
    }
    /**
     * @param string|null $lastSeen
     *
     * @return self
     */
    public function setLastSeen(?string $lastSeen): self
    {
        $this->initialized['lastSeen'] = true;
        $this->lastSeen = $lastSeen;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getRole(): ?string
    {
        return $this->role;
    }
    /**
     * @param string|null $role
     *
     * @return self
     */
    public function setRole(?string $role): self
    {
        $this->initialized['role'] = true;
        $this->role = $role;
        return $this;
    }
}