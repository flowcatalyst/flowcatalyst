<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminEventTypesIdSchemasPostBody extends \ArrayObject
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
     * @var mixed|null
     */
    protected $schemaContent;
    /**
     * @var mixed|null
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
     * @return mixed
     */
    public function getSchemaContent()
    {
        return $this->schemaContent;
    }
    /**
     * @param mixed $schemaContent
     *
     * @return self
     */
    public function setSchemaContent($schemaContent): self
    {
        $this->initialized['schemaContent'] = true;
        $this->schemaContent = $schemaContent;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getSchemaType()
    {
        return $this->schemaType;
    }
    /**
     * @param mixed $schemaType
     *
     * @return self
     */
    public function setSchemaType($schemaType): self
    {
        $this->initialized['schemaType'] = true;
        $this->schemaType = $schemaType;
        return $this;
    }
}