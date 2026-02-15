<?php

namespace FlowCatalyst\Generated\Model;

class ForbiddenResponse extends \ArrayObject
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
     * Error code
     *
     * @var string|null
     */
    protected $code;
    /**
     * Human-readable error message
     *
     * @var string|null
     */
    protected $message;
    /**
     * Required permission if applicable
     *
     * @var string|null
     */
    protected $requiredPermission;
    /**
     * Error code
     *
     * @return string|null
     */
    public function getCode(): ?string
    {
        return $this->code;
    }
    /**
     * Error code
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
     * Required permission if applicable
     *
     * @return string|null
     */
    public function getRequiredPermission(): ?string
    {
        return $this->requiredPermission;
    }
    /**
     * Required permission if applicable
     *
     * @param string|null $requiredPermission
     *
     * @return self
     */
    public function setRequiredPermission(?string $requiredPermission): self
    {
        $this->initialized['requiredPermission'] = true;
        $this->requiredPermission = $requiredPermission;
        return $this;
    }
}