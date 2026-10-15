<?php

namespace FlowCatalyst\Generated\Model;

class CreateDomainRequest extends \ArrayObject
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
     * Omitted claims a platform-owned zone (anchor only)
     *
     * @var string|null
     */
    protected $clientId;
    /**
     * The claimed hostname zone (e.g. "acme.example.com")
     *
     * @var string|null
     */
    protected $zone;
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
     * Omitted claims a platform-owned zone (anchor only)
     *
     * @return string|null
     */
    public function getClientId(): ?string
    {
        return $this->clientId;
    }
    /**
     * Omitted claims a platform-owned zone (anchor only)
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
     * The claimed hostname zone (e.g. "acme.example.com")
     *
     * @return string|null
     */
    public function getZone(): ?string
    {
        return $this->zone;
    }
    /**
     * The claimed hostname zone (e.g. "acme.example.com")
     *
     * @param string|null $zone
     *
     * @return self
     */
    public function setZone(?string $zone): self
    {
        $this->initialized['zone'] = true;
        $this->zone = $zone;
        return $this;
    }
}