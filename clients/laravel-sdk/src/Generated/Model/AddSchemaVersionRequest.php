<?php

namespace FlowCatalyst\Generated\Model;

class AddSchemaVersionRequest extends \ArrayObject
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
     * JSON schema for this version
     *
     * @var mixed|null
     */
    protected $schema;
    /**
     * JSON schema for this version
     *
     * @return mixed
     */
    public function getSchema()
    {
        return $this->schema;
    }
    /**
     * JSON schema for this version
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
}