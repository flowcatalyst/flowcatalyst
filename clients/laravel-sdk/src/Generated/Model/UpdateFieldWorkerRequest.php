<?php

namespace FlowCatalyst\Generated\Model;

use FlowCatalyst\Generated\Runtime\AdditionalAndPatternProperties;
use FlowCatalyst\Generated\Runtime\AdditionalPropertiesInterface;
class UpdateFieldWorkerRequest implements AdditionalPropertiesInterface
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
     * Changing it needs the impersonate permission; clearing it ends the sessions the worker holds as an actor
     *
     * @var bool|null
     */
    protected $canImpersonate;
    /**
     * @var string|null
     */
    protected $email;
    /**
     * @var bool|null
     */
    protected $isTest;
    /**
     * @var string|null
     */
    protected $name;
    /**
     * Omit to leave unchanged; an empty string clears it
     *
     * @var string|null
     */
    protected $phone;
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
     * Changing it needs the impersonate permission; clearing it ends the sessions the worker holds as an actor
     *
     * @return bool|null
     */
    public function getCanImpersonate(): ?bool
    {
        return $this->canImpersonate;
    }
    /**
     * Changing it needs the impersonate permission; clearing it ends the sessions the worker holds as an actor
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
     * @return bool|null
     */
    public function getIsTest(): ?bool
    {
        return $this->isTest;
    }
    /**
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
     * Omit to leave unchanged; an empty string clears it
     *
     * @return string|null
     */
    public function getPhone(): ?string
    {
        return $this->phone;
    }
    /**
     * Omit to leave unchanged; an empty string clears it
     *
     * @param string|null $phone
     *
     * @return self
     */
    public function setPhone(?string $phone): self
    {
        $this->initialized['phone'] = true;
        $this->phone = $phone;
        return $this;
    }
    public function definedProperties(): array
    {
        return ['dollarSchema' => ['$schema', 'getDollarSchema', 'setDollarSchema'], 'canImpersonate' => ['canImpersonate', 'getCanImpersonate', 'setCanImpersonate'], 'email' => ['email', 'getEmail', 'setEmail'], 'isTest' => ['isTest', 'getIsTest', 'setIsTest'], 'name' => ['name', 'getName', 'setName'], 'phone' => ['phone', 'getPhone', 'setPhone']];
    }
}