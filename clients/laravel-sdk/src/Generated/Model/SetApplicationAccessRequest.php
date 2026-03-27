<?php

namespace FlowCatalyst\Generated\Model;

class SetApplicationAccessRequest extends \ArrayObject
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
     * Application IDs to grant access to (replaces existing)
     *
     * @var list<string>|null
     */
    protected $applicationIds;
    /**
     * Application IDs to grant access to (replaces existing)
     *
     * @return list<string>|null
     */
    public function getApplicationIds(): ?array
    {
        return $this->applicationIds;
    }
    /**
     * Application IDs to grant access to (replaces existing)
     *
     * @param list<string>|null $applicationIds
     *
     * @return self
     */
    public function setApplicationIds(?array $applicationIds): self
    {
        $this->initialized['applicationIds'] = true;
        $this->applicationIds = $applicationIds;
        return $this;
    }
}