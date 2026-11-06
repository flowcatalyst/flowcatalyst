<?php

namespace FlowCatalyst\Generated\Model;

use FlowCatalyst\Generated\Runtime\AdditionalAndPatternProperties;
use FlowCatalyst\Generated\Runtime\AdditionalPropertiesInterface;
class AuthenticateCompleteRequest implements AdditionalPropertiesInterface
{
    use AdditionalAndPatternProperties;
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
     * @var mixed|null
     */
    protected $credential;
    /**
     * @var string|null
     */
    protected $stateId;
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
     * @return mixed
     */
    public function getCredential()
    {
        return $this->credential;
    }
    /**
     * @param mixed $credential
     *
     * @return self
     */
    public function setCredential($credential): self
    {
        $this->initialized['credential'] = true;
        $this->credential = $credential;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getStateId(): ?string
    {
        return $this->stateId;
    }
    /**
     * @param string|null $stateId
     *
     * @return self
     */
    public function setStateId(?string $stateId): self
    {
        $this->initialized['stateId'] = true;
        $this->stateId = $stateId;
        return $this;
    }
    public function definedProperties(): array
    {
        return ['dollarSchema' => ['$schema', 'getDollarSchema', 'setDollarSchema'], 'credential' => ['credential', 'getCredential', 'setCredential'], 'stateId' => ['stateId', 'getStateId', 'setStateId']];
    }
}