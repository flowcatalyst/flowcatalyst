<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminSubscriptionsIdGetResponse200EventTypesItem extends \ArrayObject
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
     * @var mixed|null
     */
    protected $eventTypeId;
    /**
     * @var string|null
     */
    protected $eventTypeCode;
    /**
     * @var mixed|null
     */
    protected $specVersion;
    /**
     * @return mixed
     */
    public function getEventTypeId()
    {
        return $this->eventTypeId;
    }
    /**
     * @param mixed $eventTypeId
     *
     * @return self
     */
    public function setEventTypeId($eventTypeId): self
    {
        $this->initialized['eventTypeId'] = true;
        $this->eventTypeId = $eventTypeId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getEventTypeCode(): ?string
    {
        return $this->eventTypeCode;
    }
    /**
     * @param string|null $eventTypeCode
     *
     * @return self
     */
    public function setEventTypeCode(?string $eventTypeCode): self
    {
        $this->initialized['eventTypeCode'] = true;
        $this->eventTypeCode = $eventTypeCode;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getSpecVersion()
    {
        return $this->specVersion;
    }
    /**
     * @param mixed $specVersion
     *
     * @return self
     */
    public function setSpecVersion($specVersion): self
    {
        $this->initialized['specVersion'] = true;
        $this->specVersion = $specVersion;
        return $this;
    }
}