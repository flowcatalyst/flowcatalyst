<?php

namespace FlowCatalyst\Generated\Model;

class CreateEventTypeRequest extends \ArrayObject
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
     * Client ID (optional, null = anchor-level)
     *
     * @var string|null
     */
    protected $clientId;
    /**
     * Event type code (e.g., "orders:fulfillment:shipment:shipped")
     * Format: {application}:{subdomain}:{aggregate}:{event}
     *
     * @var string|null
     */
    protected $code;
    /**
     * Description
     *
     * @var string|null
     */
    protected $description;
    /**
     * Human-readable name
     *
     * @var string|null
     */
    protected $name;
    /**
     * Initial JSON schema
     *
     * @var mixed|null
     */
    protected $schema;
    /**
     * Client ID (optional, null = anchor-level)
     *
     * @return string|null
     */
    public function getClientId(): ?string
    {
        return $this->clientId;
    }
    /**
     * Client ID (optional, null = anchor-level)
     *
     * @param string|null $clientId
     *
     * @return self
     */
    public function setClientId(?string $clientId): self
    {
        $this->initialized['clientId'] = true;
        $this->clientId = $clientId;
        return $this;
    }
    /**
     * Event type code (e.g., "orders:fulfillment:shipment:shipped")
     * Format: {application}:{subdomain}:{aggregate}:{event}
     *
     * @return string|null
     */
    public function getCode(): ?string
    {
        return $this->code;
    }
    /**
    * Event type code (e.g., "orders:fulfillment:shipment:shipped")
    Format: {application}:{subdomain}:{aggregate}:{event}
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
     * Description
     *
     * @return string|null
     */
    public function getDescription(): ?string
    {
        return $this->description;
    }
    /**
     * Description
     *
     * @param string|null $description
     *
     * @return self
     */
    public function setDescription(?string $description): self
    {
        $this->initialized['description'] = true;
        $this->description = $description;
        return $this;
    }
    /**
     * Human-readable name
     *
     * @return string|null
     */
    public function getName(): ?string
    {
        return $this->name;
    }
    /**
     * Human-readable name
     *
     * @param string|null $name
     *
     * @return self
     */
    public function setName(?string $name): self
    {
        $this->initialized['name'] = true;
        $this->name = $name;
        return $this;
    }
    /**
     * Initial JSON schema
     *
     * @return mixed
     */
    public function getSchema()
    {
        return $this->schema;
    }
    /**
     * Initial JSON schema
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