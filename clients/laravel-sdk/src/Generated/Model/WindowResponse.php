<?php

namespace FlowCatalyst\Generated\Model;

class WindowResponse
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
     * true when the server supplied a bound the request did not (no since/until: the last 24 hours)
     *
     * @var bool|null
     */
    protected $defaulted;
    /**
     * Start of the window (inclusive)
     *
     * @var \DateTime|null
     */
    protected $from;
    /**
     * End of the window; now when the request gave no until
     *
     * @var \DateTime|null
     */
    protected $to;
    /**
     * true when the server supplied a bound the request did not (no since/until: the last 24 hours)
     *
     * @return bool|null
     */
    public function getDefaulted(): ?bool
    {
        return $this->defaulted;
    }
    /**
     * true when the server supplied a bound the request did not (no since/until: the last 24 hours)
     *
     * @param bool|null $defaulted
     *
     * @return self
     */
    public function setDefaulted(?bool $defaulted): self
    {
        $this->initialized['defaulted'] = true;
        $this->defaulted = $defaulted;
        return $this;
    }
    /**
     * Start of the window (inclusive)
     *
     * @return \DateTime|null
     */
    public function getFrom(): ?\DateTime
    {
        return $this->from;
    }
    /**
     * Start of the window (inclusive)
     *
     * @param \DateTime|null $from
     *
     * @return self
     */
    public function setFrom(?\DateTime $from): self
    {
        $this->initialized['from'] = true;
        $this->from = $from;
        return $this;
    }
    /**
     * End of the window; now when the request gave no until
     *
     * @return \DateTime|null
     */
    public function getTo(): ?\DateTime
    {
        return $this->to;
    }
    /**
     * End of the window; now when the request gave no until
     *
     * @param \DateTime|null $to
     *
     * @return self
     */
    public function setTo(?\DateTime $to): self
    {
        $this->initialized['to'] = true;
        $this->to = $to;
        return $this;
    }
}