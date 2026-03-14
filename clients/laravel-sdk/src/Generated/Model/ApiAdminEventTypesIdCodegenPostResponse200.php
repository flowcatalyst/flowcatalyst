<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminEventTypesIdCodegenPostResponse200 extends \ArrayObject
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
    protected $code;
    /**
     * @var string|null
     */
    protected $language;
    /**
     * @var string|null
     */
    protected $eventTypeId;
    /**
     * @var string|null
     */
    protected $eventCode;
    /**
     * @var string|null
     */
    protected $schemaVersion;
    /**
     * @return string|null
     */
    public function getCode(): ?string
    {
        return $this->code;
    }
    /**
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
     * @return string|null
     */
    public function getLanguage(): ?string
    {
        return $this->language;
    }
    /**
     * @param string|null $language
     *
     * @return self
     */
    public function setLanguage(?string $language): self
    {
        $this->initialized['language'] = true;
        $this->language = $language;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getEventTypeId(): ?string
    {
        return $this->eventTypeId;
    }
    /**
     * @param string|null $eventTypeId
     *
     * @return self
     */
    public function setEventTypeId(?string $eventTypeId): self
    {
        $this->initialized['eventTypeId'] = true;
        $this->eventTypeId = $eventTypeId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getEventCode(): ?string
    {
        return $this->eventCode;
    }
    /**
     * @param string|null $eventCode
     *
     * @return self
     */
    public function setEventCode(?string $eventCode): self
    {
        $this->initialized['eventCode'] = true;
        $this->eventCode = $eventCode;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getSchemaVersion(): ?string
    {
        return $this->schemaVersion;
    }
    /**
     * @param string|null $schemaVersion
     *
     * @return self
     */
    public function setSchemaVersion(?string $schemaVersion): self
    {
        $this->initialized['schemaVersion'] = true;
        $this->schemaVersion = $schemaVersion;
        return $this;
    }
}