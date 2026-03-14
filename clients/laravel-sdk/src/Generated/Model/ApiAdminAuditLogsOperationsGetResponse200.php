<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminAuditLogsOperationsGetResponse200 extends \ArrayObject
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
     * @var list<string>|null
     */
    protected $operations;
    /**
     * @return list<string>|null
     */
    public function getOperations(): ?array
    {
        return $this->operations;
    }
    /**
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