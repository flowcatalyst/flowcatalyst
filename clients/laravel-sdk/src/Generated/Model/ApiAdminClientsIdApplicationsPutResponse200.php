<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminClientsIdApplicationsPutResponse200 extends \ArrayObject
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
     * Human-readable status message
     *
     * @var string|null
     */
    protected $message;
    /**
     * Human-readable status message
     *
     * @return string|null
     */
    public function getMessage(): ?string
    {
        return $this->message;
    }
    /**
     * Human-readable status message
     *
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
}