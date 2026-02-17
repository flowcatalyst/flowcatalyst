<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminClientsIdApplicationsPutBody extends \ArrayObject
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
    protected $enabledApplicationIds;
    /**
     * @return list<string>|null
     */
    public function getEnabledApplicationIds(): ?array
    {
        return $this->enabledApplicationIds;
    }
    /**
     * @param list<string>|null $enabledApplicationIds
     *
     * @return self
     */
    public function setEnabledApplicationIds(?array $enabledApplicationIds): self
    {
        $this->initialized['enabledApplicationIds'] = true;
        $this->enabledApplicationIds = $enabledApplicationIds;
        return $this;
    }
}