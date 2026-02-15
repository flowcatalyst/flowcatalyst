<?php

namespace FlowCatalyst\Generated\Model;

class UpdateClientApplicationsRequest extends \ArrayObject
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
     * List of application IDs to enable for this client
     *
     * @var list<string>|null
     */
    protected $enabledApplicationIds;
    /**
     * List of application IDs to enable for this client
     *
     * @return list<string>|null
     */
    public function getEnabledApplicationIds(): ?array
    {
        return $this->enabledApplicationIds;
    }
    /**
     * List of application IDs to enable for this client
     *
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