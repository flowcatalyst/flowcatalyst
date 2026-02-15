<?php

namespace FlowCatalyst\Generated\Model;

class NotFoundResponse extends \ArrayObject
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
     * Type of resource that was not found
     *
     * @var string|null
     */
    protected $resourceType;
    /**
     * ID that was searched for
     *
     * @var string|null
     */
    protected $resourceId;
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
     * Type of resource that was not found
     *
     * @return string|null
     */
    public function getResourceType(): ?string
    {
        return $this->resourceType;
    }
    /**
     * Type of resource that was not found
     *
     * @param string|null $resourceType
     *
     * @return self
     */
    public function setResourceType(?string $resourceType): self
    {
        $this->initialized['resourceType'] = true;
        $this->resourceType = $resourceType;
        return $this;
    }
    /**
     * ID that was searched for
     *
     * @return string|null
     */
    public function getResourceId(): ?string
    {
        return $this->resourceId;
    }
    /**
     * ID that was searched for
     *
     * @param string|null $resourceId
     *
     * @return self
     */
    public function setResourceId(?string $resourceId): self
    {
        $this->initialized['resourceId'] = true;
        $this->resourceId = $resourceId;
        return $this;
    }
}