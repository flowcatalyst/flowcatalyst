<?php

namespace FlowCatalyst\Generated\Model;

class OperationsResponse extends \ArrayObject
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
     * Distinct operations with audit logs
     *
     * @var list<string>|null
     */
    protected $operations;
    /**
     * Distinct operations with audit logs
     *
     * @return list<string>|null
     */
    public function getOperations(): ?array
    {
        return $this->operations;
    }
    /**
     * Distinct operations with audit logs
     *
     * @param list<string>|null $operations
     *
     * @return self
     */
    public function setOperations(?array $operations): self
    {
        $this->initialized['operations'] = true;
        $this->operations = $operations;
        return $this;
    }
}