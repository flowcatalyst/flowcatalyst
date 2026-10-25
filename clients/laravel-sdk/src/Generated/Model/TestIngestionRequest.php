<?php

namespace FlowCatalyst\Generated\Model;

class TestIngestionRequest extends \ArrayObject
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
     * A URL to the JSON Schema for this object.
     *
     * @var string|null
     */
    protected $dollarSchema;
    /**
     * With definition: code of the owning application, which must be of type INTEGRATION
     *
     * @var string|null
     */
    protected $application;
    /**
     * With definition: the client's id (clt_...) or identifier
     *
     * @var string|null
     */
    protected $client;
    /**
     * @var IngestionDefinition|null
     */
    protected $definition;
    /**
     * Test this saved ingestion's live definition
     *
     * @var string|null
     */
    protected $ingestionId;
    /**
     * A URL to the JSON Schema for this object.
     *
     * @return string|null
     */
    public function getDollarSchema(): ?string
    {
        return $this->dollarSchema;
    }
    /**
     * A URL to the JSON Schema for this object.
     *
     * @param string|null $dollarSchema
     *
     * @return self
     */
    public function setDollarSchema(?string $dollarSchema): self
    {
        $this->initialized['dollarSchema'] = true;
        $this->dollarSchema = $dollarSchema;
        return $this;
    }
    /**
     * With definition: code of the owning application, which must be of type INTEGRATION
     *
     * @return string|null
     */
    public function getApplication(): ?string
    {
        return $this->application;
    }
    /**
     * With definition: code of the owning application, which must be of type INTEGRATION
     *
     * @param string|null $application
     *
     * @return self
     */
    public function setApplication(?string $application): self
    {
        $this->initialized['application'] = true;
        $this->application = $application;
        return $this;
    }
    /**
     * With definition: the client's id (clt_...) or identifier
     *
     * @return string|null
     */
    public function getClient(): ?string
    {
        return $this->client;
    }
    /**
     * With definition: the client's id (clt_...) or identifier
     *
     * @param string|null $client
     *
     * @return self
     */
    public function setClient(?string $client): self
    {
        $this->initialized['client'] = true;
        $this->client = $client;
        return $this;
    }
    /**
     * @return IngestionDefinition|null
     */
    public function getDefinition(): ?IngestionDefinition
    {
        return $this->definition;
    }
    /**
     * @param IngestionDefinition|null $definition
     *
     * @return self
     */
    public function setDefinition(?IngestionDefinition $definition): self
    {
        $this->initialized['definition'] = true;
        $this->definition = $definition;
        return $this;
    }
    /**
     * Test this saved ingestion's live definition
     *
     * @return string|null
     */
    public function getIngestionId(): ?string
    {
        return $this->ingestionId;
    }
    /**
     * Test this saved ingestion's live definition
     *
     * @param string|null $ingestionId
     *
     * @return self
     */
    public function setIngestionId(?string $ingestionId): self
    {
        $this->initialized['ingestionId'] = true;
        $this->ingestionId = $ingestionId;
        return $this;
    }
}