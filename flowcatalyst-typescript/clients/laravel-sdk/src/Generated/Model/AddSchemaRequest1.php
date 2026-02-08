<?php

namespace FlowCatalyst\Generated\Model;

class AddSchemaRequest1 extends \ArrayObject
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
    protected $version;
    /**
     * @var string|null
     */
    protected $mimeType;
    /**
     * @var string|null
     */
    protected $schema;
    /**
     * @var string|null
     */
    protected $schemaType;
    /**
     * @return string|null
     */
    public function getVersion(): ?string
    {
        return $this->version;
    }
    /**
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
    /**
     * @return string|null
     */
    public function getMimeType(): ?string
    {
        return $this->mimeType;
    }
    /**
     * @param string|null $mimeType
     *
     * @return self
     */
    public function setMimeType(?string $mimeType): self
    {
        $this->initialized['mimeType'] = true;
        $this->mimeType = $mimeType;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getSchema(): ?string
    {
        return $this->schema;
    }
    /**
     * @param string|null $schema
     *
     * @return self
     */
    public function setSchema(?string $schema): self
    {
        $this->initialized['schema'] = true;
        $this->schema = $schema;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getSchemaType(): ?string
    {
        return $this->schemaType;
    }
    /**
     * @param string|null $schemaType
     *
     * @return self
     */
    public function setSchemaType(?string $schemaType): self
    {
        $this->initialized['schemaType'] = true;
        $this->schemaType = $schemaType;
        return $this;
    }
}