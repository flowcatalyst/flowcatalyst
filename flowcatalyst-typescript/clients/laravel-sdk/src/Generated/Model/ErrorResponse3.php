<?php

namespace FlowCatalyst\Generated\Model;

class ErrorResponse3 extends \ArrayObject
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
    protected $error;
    /**
     * @return string|null
     */
    public function getError(): ?string
    {
        return $this->error;
    }
    /**
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
}