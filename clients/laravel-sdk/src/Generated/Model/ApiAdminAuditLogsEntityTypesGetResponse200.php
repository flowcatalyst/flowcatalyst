<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminAuditLogsEntityTypesGetResponse200 extends \ArrayObject
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
    protected $entityTypes;
    /**
     * @return list<string>|null
     */
    public function getEntityTypes(): ?array
    {
        return $this->entityTypes;
    }
    /**
     * @param list<string>|null $entityTypes
     *
     * @return self
     */
    public function setEntityTypes(?array $entityTypes): self
    {
        $this->initialized['entityTypes'] = true;
        $this->entityTypes = $entityTypes;
        return $this;
    }
}