<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminConfigAppCodeSectionPropertyGetResponse200 extends \ArrayObject
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
    protected $applicationCode;
    /**
     * @var string|null
     */
    protected $section;
    /**
     * @var string|null
     */
    protected $property;
    /**
     * @var string|null
     */
    protected $scope;
    /**
     * @var mixed|null
     */
    protected $clientId;
    /**
     * @var string|null
     */
    protected $value;
    /**
     * @return string|null
     */
    public function getApplicationCode(): ?string
    {
        return $this->applicationCode;
    }
    /**
     * @param string|null $applicationCode
     *
     * @return self
     */
    public function setApplicationCode(?string $applicationCode): self
    {
        $this->initialized['applicationCode'] = true;
        $this->applicationCode = $applicationCode;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getSection(): ?string
    {
        return $this->section;
    }
    /**
     * @param string|null $section
     *
     * @return self
     */
    public function setSection(?string $section): self
    {
        $this->initialized['section'] = true;
        $this->section = $section;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getProperty(): ?string
    {
        return $this->property;
    }
    /**
     * @param string|null $property
     *
     * @return self
     */
    public function setProperty(?string $property): self
    {
        $this->initialized['property'] = true;
        $this->property = $property;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getScope(): ?string
    {
        return $this->scope;
    }
    /**
     * @param string|null $scope
     *
     * @return self
     */
    public function setScope(?string $scope): self
    {
        $this->initialized['scope'] = true;
        $this->scope = $scope;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getClientId()
    {
        return $this->clientId;
    }
    /**
     * @param mixed $clientId
     *
     * @return self
     */
    public function setClientId($clientId): self
    {
        $this->initialized['clientId'] = true;
        $this->clientId = $clientId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getValue(): ?string
    {
        return $this->value;
    }
    /**
     * @param string|null $value
     *
     * @return self
     */
    public function setValue(?string $value): self
    {
        $this->initialized['value'] = true;
        $this->value = $value;
        return $this;
    }
}