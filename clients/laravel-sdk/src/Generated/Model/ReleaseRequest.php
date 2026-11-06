<?php

namespace FlowCatalyst\Generated\Model;

use FlowCatalyst\Generated\Runtime\AdditionalAndPatternProperties;
use FlowCatalyst\Generated\Runtime\AdditionalPropertiesInterface;
class ReleaseRequest implements AdditionalPropertiesInterface
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
     * The runtime is shutting down: it stops counting as live at once, for fair shares and for the instance guard.
     *
     * @var bool|null
     */
    protected $goodbye;
    /**
     * Single-owner ingestions whose routes have stopped; their leases end now. One the runtime does not hold is ignored.
     *
     * @var list<string>|null
     */
    protected $ingestionIds;
    /**
     * The runtime process, as on poll. When it differs from the instance stored for runtimeId the release is refused with 409 RUNTIME_ID_IN_USE and changes nothing.
     *
     * @var string|null
     */
    protected $instanceId;
    /**
     * The releasing runtime
     *
     * @var string|null
     */
    protected $runtimeId;
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
     * The runtime is shutting down: it stops counting as live at once, for fair shares and for the instance guard.
     *
     * @return bool|null
     */
    public function getGoodbye(): ?bool
    {
        return $this->goodbye;
    }
    /**
     * The runtime is shutting down: it stops counting as live at once, for fair shares and for the instance guard.
     *
     * @param bool|null $goodbye
     *
     * @return self
     */
    public function setGoodbye(?bool $goodbye): self
    {
        $this->initialized['goodbye'] = true;
        $this->goodbye = $goodbye;
        return $this;
    }
    /**
     * Single-owner ingestions whose routes have stopped; their leases end now. One the runtime does not hold is ignored.
     *
     * @return list<string>|null
     */
    public function getIngestionIds(): ?array
    {
        return $this->ingestionIds;
    }
    /**
     * Single-owner ingestions whose routes have stopped; their leases end now. One the runtime does not hold is ignored.
     *
     * @param list<string>|null $ingestionIds
     *
     * @return self
     */
    public function setIngestionIds(?array $ingestionIds): self
    {
        $this->initialized['ingestionIds'] = true;
        $this->ingestionIds = $ingestionIds;
        return $this;
    }
    /**
     * The runtime process, as on poll. When it differs from the instance stored for runtimeId the release is refused with 409 RUNTIME_ID_IN_USE and changes nothing.
     *
     * @return string|null
     */
    public function getInstanceId(): ?string
    {
        return $this->instanceId;
    }
    /**
     * The runtime process, as on poll. When it differs from the instance stored for runtimeId the release is refused with 409 RUNTIME_ID_IN_USE and changes nothing.
     *
     * @param string|null $instanceId
     *
     * @return self
     */
    public function setInstanceId(?string $instanceId): self
    {
        $this->initialized['instanceId'] = true;
        $this->instanceId = $instanceId;
        return $this;
    }
    /**
     * The releasing runtime
     *
     * @return string|null
     */
    public function getRuntimeId(): ?string
    {
        return $this->runtimeId;
    }
    /**
     * The releasing runtime
     *
     * @param string|null $runtimeId
     *
     * @return self
     */
    public function setRuntimeId(?string $runtimeId): self
    {
        $this->initialized['runtimeId'] = true;
        $this->runtimeId = $runtimeId;
        return $this;
    }
    public function definedProperties(): array
    {
        return ['dollarSchema' => ['$schema', 'getDollarSchema', 'setDollarSchema'], 'goodbye' => ['goodbye', 'getGoodbye', 'setGoodbye'], 'ingestionIds' => ['ingestionIds', 'getIngestionIds', 'setIngestionIds'], 'instanceId' => ['instanceId', 'getInstanceId', 'setInstanceId'], 'runtimeId' => ['runtimeId', 'getRuntimeId', 'setRuntimeId']];
    }
}