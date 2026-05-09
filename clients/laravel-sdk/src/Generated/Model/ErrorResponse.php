<?php

namespace FlowCatalyst\Generated\Model;

class ErrorResponse extends \ArrayObject
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
     * Machine-readable error code (e.g. ROLE_HAS_ASSIGNMENTS)
     *
     * @var string|null
     */
    protected $error;
    /**
     * Human-readable error message suitable for display
     *
     * @var string|null
     */
    protected $message;
    /**
     * Machine-readable error code (e.g. ROLE_HAS_ASSIGNMENTS)
     *
     * @return string|null
     */
    public function getError(): ?string
    {
        return $this->error;
    }
    /**
     * Machine-readable error code (e.g. ROLE_HAS_ASSIGNMENTS)
     *
     * @param string|null $error
     *
     * @return self
     */
    public function setError(?string $error): self
    {
        $this->initialized['error'] = true;
        $this->error = $error;
        return $this;
    }
    /**
     * Human-readable error message suitable for display
     *
     * @return string|null
     */
    public function getMessage(): ?string
    {
        return $this->message;
    }
    /**
     * Human-readable error message suitable for display
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