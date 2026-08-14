<?php

namespace FlowCatalyst\Generated\Model;

class PortalUserListResponse
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
     * A URL to the JSON Schema for this object.
     *
     * @var string|null
     */
    protected $dollarSchema;
    /**
     * @var list<PortalUserListItem>|null
     */
    protected $portalUsers;
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
     * @return list<PortalUserListItem>|null
     */
    public function getPortalUsers(): ?array
    {
        return $this->portalUsers;
    }
    /**
     * @param list<PortalUserListItem>|null $portalUsers
     *
     * @return self
     */
    public function setPortalUsers(?array $portalUsers): self
    {
        $this->initialized['portalUsers'] = true;
        $this->portalUsers = $portalUsers;
        return $this;
    }
}