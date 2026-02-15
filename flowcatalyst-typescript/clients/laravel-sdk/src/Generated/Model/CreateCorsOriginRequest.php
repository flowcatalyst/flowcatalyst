<?php

namespace FlowCatalyst\Generated\Model;

class CreateCorsOriginRequest extends \ArrayObject
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
     * Origin URL
     *
     * @var string|null
     */
    protected $origin;
    /**
     * Description of why this origin is allowed
     *
     * @var string|null
     */
    protected $description;
    /**
     * Origin URL
     *
     * @return string|null
     */
    public function getOrigin(): ?string
    {
        return $this->origin;
    }
    /**
     * Origin URL
     *
     * @param string|null $origin
     *
     * @return self
     */
    public function setOrigin(?string $origin): self
    {
        $this->initialized['origin'] = true;
        $this->origin = $origin;
        return $this;
    }
    /**
     * Description of why this origin is allowed
     *
     * @return string|null
     */
    public function getDescription(): ?string
    {
        return $this->description;
    }
    /**
     * Description of why this origin is allowed
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
}