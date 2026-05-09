<?php

namespace FlowCatalyst\Generated\Model;

class StandbyStatus extends \ArrayObject
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
     * Cluster members
     *
     * @var list<ClusterMember>|null
     */
    protected $clusterMembers;
    /**
     * Instance ID
     *
     * @var string|null
     */
    protected $instanceId;
    /**
     * Whether this instance is the leader
     *
     * @var bool|null
     */
    protected $isLeader;
    /**
     * Last heartbeat time
     *
     * @var string|null
     */
    protected $lastHeartbeat;
    /**
     * Leader instance ID (if known)
     *
     * @var string|null
     */
    protected $leaderId;
    /**
     * Current role (LEADER or STANDBY)
     *
     * @var string|null
     */
    protected $role;
    /**
     * Cluster members
     *
     * @return list<ClusterMember>|null
     */
    public function getClusterMembers(): ?array
    {
        return $this->clusterMembers;
    }
    /**
     * Cluster members
     *
     * @param list<ClusterMember>|null $clusterMembers
     *
     * @return self
     */
    public function setClusterMembers(?array $clusterMembers): self
    {
        $this->initialized['clusterMembers'] = true;
        $this->clusterMembers = $clusterMembers;
        return $this;
    }
    /**
     * Instance ID
     *
     * @return string|null
     */
    public function getInstanceId(): ?string
    {
        return $this->instanceId;
    }
    /**
     * Instance ID
     *
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
     * Whether this instance is the leader
     *
     * @return bool|null
     */
    public function getIsLeader(): ?bool
    {
        return $this->isLeader;
    }
    /**
     * Whether this instance is the leader
     *
     * @param bool|null $isLeader
     *
     * @return self
     */
    public function setIsLeader(?bool $isLeader): self
    {
        $this->initialized['isLeader'] = true;
        $this->isLeader = $isLeader;
        return $this;
    }
    /**
     * Last heartbeat time
     *
     * @return string|null
     */
    public function getLastHeartbeat(): ?string
    {
        return $this->lastHeartbeat;
    }
    /**
     * Last heartbeat time
     *
     * @param string|null $lastHeartbeat
     *
     * @return self
     */
    public function setLastHeartbeat(?string $lastHeartbeat): self
    {
        $this->initialized['lastHeartbeat'] = true;
        $this->lastHeartbeat = $lastHeartbeat;
        return $this;
    }
    /**
     * Leader instance ID (if known)
     *
     * @return string|null
     */
    public function getLeaderId(): ?string
    {
        return $this->leaderId;
    }
    /**
     * Leader instance ID (if known)
     *
     * @param string|null $leaderId
     *
     * @return self
     */
    public function setLeaderId(?string $leaderId): self
    {
        $this->initialized['leaderId'] = true;
        $this->leaderId = $leaderId;
        return $this;
    }
    /**
     * Current role (LEADER or STANDBY)
     *
     * @return string|null
     */
    public function getRole(): ?string
    {
        return $this->role;
    }
    /**
     * Current role (LEADER or STANDBY)
     *
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