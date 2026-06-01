<?php

namespace FlowCatalyst\Generated\Model;

class DashboardMetrics extends \ArrayObject
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
     * Active dispatch pools
     *
     * @var int|null
     */
    protected $activePools;
    /**
     * Active subscriptions
     *
     * @var int|null
     */
    protected $activeSubscriptions;
    /**
     * Events in last hour. Currently always 0 — needs a time-windowed
     * counter on the projection or an external metrics store.
     *
     * @var int|null
     */
    protected $eventsLastHour;
    /**
     * System health info
     *
     * @var SystemHealth|null
     */
    protected $health;
    /**
     * Approximate total events received (planner estimate).
     *
     * @var int|null
     */
    protected $totalEvents;
    /**
     * Approximate total dispatch jobs (planner estimate).
     *
     * @var int|null
     */
    protected $totalJobs;
    /**
     * Active dispatch pools
     *
     * @return int|null
     */
    public function getActivePools(): ?int
    {
        return $this->activePools;
    }
    /**
     * Active dispatch pools
     *
     * @param int|null $activePools
     *
     * @return self
     */
    public function setActivePools(?int $activePools): self
    {
        $this->initialized['activePools'] = true;
        $this->activePools = $activePools;
        return $this;
    }
    /**
     * Active subscriptions
     *
     * @return int|null
     */
    public function getActiveSubscriptions(): ?int
    {
        return $this->activeSubscriptions;
    }
    /**
     * Active subscriptions
     *
     * @param int|null $activeSubscriptions
     *
     * @return self
     */
    public function setActiveSubscriptions(?int $activeSubscriptions): self
    {
        $this->initialized['activeSubscriptions'] = true;
        $this->activeSubscriptions = $activeSubscriptions;
        return $this;
    }
    /**
     * Events in last hour. Currently always 0 — needs a time-windowed
     * counter on the projection or an external metrics store.
     *
     * @return int|null
     */
    public function getEventsLastHour(): ?int
    {
        return $this->eventsLastHour;
    }
    /**
    * Events in last hour. Currently always 0 — needs a time-windowed
    counter on the projection or an external metrics store.
    *
    * @param int|null $eventsLastHour
    *
    * @return self
    */
    public function setEventsLastHour(?int $eventsLastHour): self
    {
        $this->initialized['eventsLastHour'] = true;
        $this->eventsLastHour = $eventsLastHour;
        return $this;
    }
    /**
     * System health info
     *
     * @return SystemHealth|null
     */
    public function getHealth(): ?SystemHealth
    {
        return $this->health;
    }
    /**
     * System health info
     *
     * @param SystemHealth|null $health
     *
     * @return self
     */
    public function setHealth(?SystemHealth $health): self
    {
        $this->initialized['health'] = true;
        $this->health = $health;
        return $this;
    }
    /**
     * Approximate total events received (planner estimate).
     *
     * @return int|null
     */
    public function getTotalEvents(): ?int
    {
        return $this->totalEvents;
    }
    /**
     * Approximate total events received (planner estimate).
     *
     * @param int|null $totalEvents
     *
     * @return self
     */
    public function setTotalEvents(?int $totalEvents): self
    {
        $this->initialized['totalEvents'] = true;
        $this->totalEvents = $totalEvents;
        return $this;
    }
    /**
     * Approximate total dispatch jobs (planner estimate).
     *
     * @return int|null
     */
    public function getTotalJobs(): ?int
    {
        return $this->totalJobs;
    }
    /**
     * Approximate total dispatch jobs (planner estimate).
     *
     * @param int|null $totalJobs
     *
     * @return self
     */
    public function setTotalJobs(?int $totalJobs): self
    {
        $this->initialized['totalJobs'] = true;
        $this->totalJobs = $totalJobs;
        return $this;
    }
}