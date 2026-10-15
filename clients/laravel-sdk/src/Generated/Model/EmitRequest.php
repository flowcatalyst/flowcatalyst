<?php

namespace FlowCatalyst\Generated\Model;

class EmitRequest extends \ArrayObject
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
     * @var string|null
     */
    protected $data;
    /**
     * @var Event|null
     */
    protected $event;
    /**
     * @var string|null
     */
    protected $functionId;
    /**
     * @var int|null
     */
    protected $version;
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
     * @return string|null
     */
    public function getData(): ?string
    {
        return $this->data;
    }
    /**
     * @param string|null $data
     *
     * @return self
     */
    public function setData(?string $data): self
    {
        $this->initialized['data'] = true;
        $this->data = $data;
        return $this;
    }
    /**
     * @return Event|null
     */
    public function getEvent(): ?Event
    {
        return $this->event;
    }
    /**
     * @param Event|null $event
     *
     * @return self
     */
    public function setEvent(?Event $event): self
    {
        $this->initialized['event'] = true;
        $this->event = $event;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getFunctionId(): ?string
    {
        return $this->functionId;
    }
    /**
     * @param string|null $functionId
     *
     * @return self
     */
    public function setFunctionId(?string $functionId): self
    {
        $this->initialized['functionId'] = true;
        $this->functionId = $functionId;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getVersion(): ?int
    {
        return $this->version;
    }
    /**
     * @param int|null $version
     *
     * @return self
     */
    public function setVersion(?int $version): self
    {
        $this->initialized['version'] = true;
        $this->version = $version;
        return $this;
    }
}