<?php

namespace FlowCatalyst\Generated\Model;

class PrunedVersionResponse
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
     * @var string|null
     */
    protected $digest;
    /**
     * @var int|null
     */
    protected $number;
    /**
     * @return string|null
     */
    public function getDigest(): ?string
    {
        return $this->digest;
    }
    /**
     * @param string|null $digest
     *
     * @return self
     */
    public function setDigest(?string $digest): self
    {
        $this->initialized['digest'] = true;
        $this->digest = $digest;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getNumber(): ?int
    {
        return $this->number;
    }
    /**
     * @param int|null $number
     *
     * @return self
     */
    public function setNumber(?int $number): self
    {
        $this->initialized['number'] = true;
        $this->number = $number;
        return $this;
    }
}