<?php

namespace FlowCatalyst\Generated\Model;

class ProcessResponse extends \ArrayObject
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
    protected $ack;
    /**
     * @var string|null
     */
    protected $message;
    /**
     * @var int|null
     */
    protected $delaySeconds;
    /**
     * @return bool|null
     */
    public function getAck(): ?bool
    {
        return $this->ack;
    }
    /**
     * @param bool|null $ack
     *
     * @return self
     */
    public function setAck(?bool $ack): self
    {
        $this->initialized['ack'] = true;
        $this->ack = $ack;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getMessage(): ?string
    {
        return $this->message;
    }
    /**
     * @param string|null $message
     *
     * @return self
     */
    public function setMessage(?string $message): self
    {
        $this->initialized['message'] = true;
        $this->message = $message;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getDelaySeconds(): ?int
    {
        return $this->delaySeconds;
    }
    /**
     * @param int|null $delaySeconds
     *
     * @return self
     */
    public function setDelaySeconds(?int $delaySeconds): self
    {
        $this->initialized['delaySeconds'] = true;
        $this->delaySeconds = $delaySeconds;
        return $this;
    }
}