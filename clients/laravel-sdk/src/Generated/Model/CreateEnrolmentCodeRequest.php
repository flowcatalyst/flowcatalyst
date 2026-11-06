<?php

namespace FlowCatalyst\Generated\Model;

use FlowCatalyst\Generated\Runtime\AdditionalAndPatternProperties;
use FlowCatalyst\Generated\Runtime\AdditionalPropertiesInterface;
class CreateEnrolmentCodeRequest implements AdditionalPropertiesInterface
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
     * @var list<string>|null
     */
    protected $appCodes;
    /**
     * PERSONAL devices only
     *
     * @var string|null
     */
    protected $assignedWorkerId;
    /**
     * @var string|null
     */
    protected $clientId;
    /**
     * @var string|null
     */
    protected $label;
    /**
     * @var string|null
     */
    protected $mode;
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
     * @return list<string>|null
     */
    public function getAppCodes(): ?array
    {
        return $this->appCodes;
    }
    /**
     * @param list<string>|null $appCodes
     *
     * @return self
     */
    public function setAppCodes(?array $appCodes): self
    {
        $this->initialized['appCodes'] = true;
        $this->appCodes = $appCodes;
        return $this;
    }
    /**
     * PERSONAL devices only
     *
     * @return string|null
     */
    public function getAssignedWorkerId(): ?string
    {
        return $this->assignedWorkerId;
    }
    /**
     * PERSONAL devices only
     *
     * @param string|null $assignedWorkerId
     *
     * @return self
     */
    public function setAssignedWorkerId(?string $assignedWorkerId): self
    {
        $this->initialized['assignedWorkerId'] = true;
        $this->assignedWorkerId = $assignedWorkerId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getClientId(): ?string
    {
        return $this->clientId;
    }
    /**
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
     * @return string|null
     */
    public function getLabel(): ?string
    {
        return $this->label;
    }
    /**
     * @param string|null $label
     *
     * @return self
     */
    public function setLabel(?string $label): self
    {
        $this->initialized['label'] = true;
        $this->label = $label;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getMode(): ?string
    {
        return $this->mode;
    }
    /**
     * @param string|null $mode
     *
     * @return self
     */
    public function setMode(?string $mode): self
    {
        $this->initialized['mode'] = true;
        $this->mode = $mode;
        return $this;
    }
    public function definedProperties(): array
    {
        return ['dollarSchema' => ['$schema', 'getDollarSchema', 'setDollarSchema'], 'appCodes' => ['appCodes', 'getAppCodes', 'setAppCodes'], 'assignedWorkerId' => ['assignedWorkerId', 'getAssignedWorkerId', 'setAssignedWorkerId'], 'clientId' => ['clientId', 'getClientId', 'setClientId'], 'label' => ['label', 'getLabel', 'setLabel'], 'mode' => ['mode', 'getMode', 'setMode']];
    }
}