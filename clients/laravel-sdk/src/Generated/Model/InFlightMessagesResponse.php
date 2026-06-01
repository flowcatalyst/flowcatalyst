<?php

namespace FlowCatalyst\Generated\Model;

class InFlightMessagesResponse extends \ArrayObject
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
     * @var array<string, int>|null
     */
    protected $byMessageGroup;
    /**
     * @var array<string, int>|null
     */
    protected $byPool;
    /**
     * @var list<InFlightMessage>|null
     */
    protected $messages;
    /**
     * @var int|null
     */
    protected $totalInFlight;
    /**
     * @return array<string, int>|null
     */
    public function getByMessageGroup(): ?iterable
    {
        return $this->byMessageGroup;
    }
    /**
     * @param array<string, int>|null $byMessageGroup
     *
     * @return self
     */
    public function setByMessageGroup(?iterable $byMessageGroup): self
    {
        $this->initialized['byMessageGroup'] = true;
        $this->byMessageGroup = $byMessageGroup;
        return $this;
    }
    /**
     * @return array<string, int>|null
     */
    public function getByPool(): ?iterable
    {
        return $this->byPool;
    }
    /**
     * @param array<string, int>|null $byPool
     *
     * @return self
     */
    public function setByPool(?iterable $byPool): self
    {
        $this->initialized['byPool'] = true;
        $this->byPool = $byPool;
        return $this;
    }
    /**
     * @return list<InFlightMessage>|null
     */
    public function getMessages(): ?array
    {
        return $this->messages;
    }
    /**
     * @param list<InFlightMessage>|null $messages
     *
     * @return self
     */
    public function setMessages(?array $messages): self
    {
        $this->initialized['messages'] = true;
        $this->messages = $messages;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getTotalInFlight(): ?int
    {
        return $this->totalInFlight;
    }
    /**
     * @param int|null $totalInFlight
     *
     * @return self
     */
    public function setTotalInFlight(?int $totalInFlight): self
    {
        $this->initialized['totalInFlight'] = true;
        $this->totalInFlight = $totalInFlight;
        return $this;
    }
}