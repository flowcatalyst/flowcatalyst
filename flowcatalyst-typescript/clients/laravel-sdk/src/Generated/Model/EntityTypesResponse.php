<?php

namespace FlowCatalyst\Generated\Model;

class EntityTypesResponse extends \ArrayObject
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
     * Distinct entity types with audit logs
     *
     * @var list<string>|null
     */
    protected $entityTypes;
    /**
     * Distinct entity types with audit logs
     *
     * @return list<string>|null
     */
    public function getEntityTypes(): ?array
    {
        return $this->entityTypes;
    }
    /**
     * Distinct entity types with audit logs
     *
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