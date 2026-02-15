<?php

namespace FlowCatalyst\Generated\Model;

class DeleteResponse extends \ArrayObject
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
     * ID of the deleted resource
     *
     * @var string|null
     */
    protected $id;
    /**
     * Type of resource deleted
     *
     * @var string|null
     */
    protected $resourceType;
    /**
     * Human-readable message
     *
     * @var string|null
     */
    protected $message;
    /**
     * ID of the deleted resource
     *
     * @return string|null
     */
    public function getId(): ?string
    {
        return $this->id;
    }
    /**
     * ID of the deleted resource
     *
     * @param string|null $id
     *
     * @return self
     */
    public function setId(?string $id): self
    {
        $this->initialized['id'] = true;
        $this->id = $id;
        return $this;
    }
    /**
     * Type of resource deleted
     *
     * @return string|null
     */
    public function getResourceType(): ?string
    {
        return $this->resourceType;
    }
    /**
     * Type of resource deleted
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
     * Human-readable message
     *
     * @return string|null
     */
    public function getMessage(): ?string
    {
        return $this->message;
    }
    /**
     * Human-readable message
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