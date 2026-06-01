<?php

namespace FlowCatalyst\Generated\Model;

class CreateClientRequest extends \ArrayObject
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
     * Unique identifier/slug (URL-safe)
     *
     * @var string|null
     */
    protected $identifier;
    /**
     * Human-readable name
     *
     * @var string|null
     */
    protected $name;
    /**
     * Unique identifier/slug (URL-safe)
     *
     * @return string|null
     */
    public function getIdentifier(): ?string
    {
        return $this->identifier;
    }
    /**
     * Unique identifier/slug (URL-safe)
     *
     * @param string|null $identifier
     *
     * @return self
     */
    public function setIdentifier(?string $identifier): self
    {
        $this->initialized['identifier'] = true;
        $this->identifier = $identifier;
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
}