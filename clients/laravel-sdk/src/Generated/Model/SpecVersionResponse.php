<?php

namespace FlowCatalyst\Generated\Model;

class SpecVersionResponse extends \ArrayObject
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
     * Schema content (included for detail views)
     *
     * @var mixed|null
     */
    protected $schema;
    /**
     * @var string|null
     */
    protected $status;
    /**
     * Version string (converted from u32 to "X.0" format for frontend compatibility)
     *
     * @var string|null
     */
    protected $version;
    /**
     * Schema content (included for detail views)
     *
     * @return mixed
     */
    public function getSchema()
    {
        return $this->schema;
    }
    /**
     * Schema content (included for detail views)
     *
     * @param mixed $schema
     *
     * @return self
     */
    public function setSchema($schema): self
    {
        $this->initialized['schema'] = true;
        $this->schema = $schema;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getStatus(): ?string
    {
        return $this->status;
    }
    /**
     * @param string|null $status
     *
     * @return self
     */
    public function setStatus(?string $status): self
    {
        $this->initialized['status'] = true;
        $this->status = $status;
        return $this;
    }
    /**
     * Version string (converted from u32 to "X.0" format for frontend compatibility)
     *
     * @return string|null
     */
    public function getVersion(): ?string
    {
        return $this->version;
    }
    /**
     * Version string (converted from u32 to "X.0" format for frontend compatibility)
     *
     * @param string|null $version
     *
     * @return self
     */
    public function setVersion(?string $version): self
    {
        $this->initialized['version'] = true;
        $this->version = $version;
        return $this;
    }
}