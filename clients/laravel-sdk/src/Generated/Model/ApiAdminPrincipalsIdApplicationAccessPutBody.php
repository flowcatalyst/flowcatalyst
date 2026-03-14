<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminPrincipalsIdApplicationAccessPutBody extends \ArrayObject
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
    protected $applicationIds;
    /**
     * @return list<string>|null
     */
    public function getApplicationIds(): ?array
    {
        return $this->applicationIds;
    }
    /**
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