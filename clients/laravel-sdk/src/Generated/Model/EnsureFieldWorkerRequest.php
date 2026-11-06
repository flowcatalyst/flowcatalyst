<?php

namespace FlowCatalyst\Generated\Model;

use FlowCatalyst\Generated\Runtime\AdditionalAndPatternProperties;
use FlowCatalyst\Generated\Runtime\AdditionalPropertiesInterface;
class EnsureFieldWorkerRequest implements AdditionalPropertiesInterface
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
     * Applications the worker may sign in to; granted additively
     *
     * @var list<string>|null
     */
    protected $appCodes;
    /**
     * New worker only; needs the impersonate permission
     *
     * @var bool|null
     */
    protected $canImpersonate;
    /**
     * @var string|null
     */
    protected $clientId;
    /**
     * @var string|null
     */
    protected $email;
    /**
     * New worker only: a test identity
     *
     * @var bool|null
     */
    protected $isTest;
    /**
     * @var string|null
     */
    protected $name;
    /**
     * @var string|null
     */
    protected $pin;
    /**
     * @var string|null
     */
    protected $workerCode;
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
     * Applications the worker may sign in to; granted additively
     *
     * @return list<string>|null
     */
    public function getAppCodes(): ?array
    {
        return $this->appCodes;
    }
    /**
     * Applications the worker may sign in to; granted additively
     *
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
     * New worker only; needs the impersonate permission
     *
     * @return bool|null
     */
    public function getCanImpersonate(): ?bool
    {
        return $this->canImpersonate;
    }
    /**
     * New worker only; needs the impersonate permission
     *
     * @param bool|null $canImpersonate
     *
     * @return self
     */
    public function setCanImpersonate(?bool $canImpersonate): self
    {
        $this->initialized['canImpersonate'] = true;
        $this->canImpersonate = $canImpersonate;
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
    public function getEmail(): ?string
    {
        return $this->email;
    }
    /**
     * @param string|null $email
     *
     * @return self
     */
    public function setEmail(?string $email): self
    {
        $this->initialized['email'] = true;
        $this->email = $email;
        return $this;
    }
    /**
     * New worker only: a test identity
     *
     * @return bool|null
     */
    public function getIsTest(): ?bool
    {
        return $this->isTest;
    }
    /**
     * New worker only: a test identity
     *
     * @param bool|null $isTest
     *
     * @return self
     */
    public function setIsTest(?bool $isTest): self
    {
        $this->initialized['isTest'] = true;
        $this->isTest = $isTest;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getName(): ?string
    {
        return $this->name;
    }
    /**
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
     * @return string|null
     */
    public function getPin(): ?string
    {
        return $this->pin;
    }
    /**
     * @param string|null $pin
     *
     * @return self
     */
    public function setPin(?string $pin): self
    {
        $this->initialized['pin'] = true;
        $this->pin = $pin;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getWorkerCode(): ?string
    {
        return $this->workerCode;
    }
    /**
     * @param string|null $workerCode
     *
     * @return self
     */
    public function setWorkerCode(?string $workerCode): self
    {
        $this->initialized['workerCode'] = true;
        $this->workerCode = $workerCode;
        return $this;
    }
    public function definedProperties(): array
    {
        return ['dollarSchema' => ['$schema', 'getDollarSchema', 'setDollarSchema'], 'appCodes' => ['appCodes', 'getAppCodes', 'setAppCodes'], 'canImpersonate' => ['canImpersonate', 'getCanImpersonate', 'setCanImpersonate'], 'clientId' => ['clientId', 'getClientId', 'setClientId'], 'email' => ['email', 'getEmail', 'setEmail'], 'isTest' => ['isTest', 'getIsTest', 'setIsTest'], 'name' => ['name', 'getName', 'setName'], 'pin' => ['pin', 'getPin', 'setPin'], 'workerCode' => ['workerCode', 'getWorkerCode', 'setWorkerCode']];
    }
}