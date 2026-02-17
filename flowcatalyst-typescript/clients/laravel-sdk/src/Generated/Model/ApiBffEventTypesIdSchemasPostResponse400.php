<?php

namespace FlowCatalyst\Generated\Model;

class ApiBffEventTypesIdSchemasPostResponse400 extends \ArrayObject
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
     * Human-readable error message
     *
     * @var string|null
     */
    protected $message;
    /**
     * Machine-readable error code
     *
     * @var string|null
     */
    protected $code;
    /**
     * Additional error details
     *
     * @var array<string, mixed>|null
     */
    protected $details;
    /**
     * Human-readable error message
     *
     * @return string|null
     */
    public function getMessage(): ?string
    {
        return $this->message;
    }
    /**
     * Human-readable error message
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
    /**
     * Machine-readable error code
     *
     * @return string|null
     */
    public function getCode(): ?string
    {
        return $this->code;
    }
    /**
     * Machine-readable error code
     *
     * @param string|null $code
     *
     * @return self
     */
    public function setCode(?string $code): self
    {
        $this->initialized['code'] = true;
        $this->code = $code;
        return $this;
    }
    /**
     * Additional error details
     *
     * @return array<string, mixed>|null
     */
    public function getDetails(): ?iterable
    {
        return $this->details;
    }
    /**
     * Additional error details
     *
     * @param array<string, mixed>|null $details
     *
     * @return self
     */
    public function setDetails(?iterable $details): self
    {
        $this->initialized['details'] = true;
        $this->details = $details;
        return $this;
    }
}