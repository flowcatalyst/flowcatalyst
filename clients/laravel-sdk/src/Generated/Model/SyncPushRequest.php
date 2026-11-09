<?php

namespace FlowCatalyst\Generated\Model;

use FlowCatalyst\Generated\Runtime\AdditionalAndPatternProperties;
use FlowCatalyst\Generated\Runtime\AdditionalPropertiesInterface;
class SyncPushRequest implements AdditionalPropertiesInterface
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
     * @var SyncPushKeysRequest|null
     */
    protected $keys;
    /**
     * @var list<SyncPushRuleRequest>|null
     */
    protected $rules;
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
     * @return SyncPushKeysRequest|null
     */
    public function getKeys(): ?SyncPushKeysRequest
    {
        return $this->keys;
    }
    /**
     * @param SyncPushKeysRequest|null $keys
     *
     * @return self
     */
    public function setKeys(?SyncPushKeysRequest $keys): self
    {
        $this->initialized['keys'] = true;
        $this->keys = $keys;
        return $this;
    }
    /**
     * @return list<SyncPushRuleRequest>|null
     */
    public function getRules(): ?array
    {
        return $this->rules;
    }
    /**
     * @param list<SyncPushRuleRequest>|null $rules
     *
     * @return self
     */
    public function setRules(?array $rules): self
    {
        $this->initialized['rules'] = true;
        $this->rules = $rules;
        return $this;
    }
    public function definedProperties(): array
    {
        return ['dollarSchema' => ['$schema', 'getDollarSchema', 'setDollarSchema'], 'keys' => ['keys', 'getKeys', 'setKeys'], 'rules' => ['rules', 'getRules', 'setRules']];
    }
}