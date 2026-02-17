<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminEventTypesSyncPostBody extends \ArrayObject
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
     * @var string|null
     */
    protected $applicationCode;
    /**
     * @var list<ApiAdminEventTypesSyncPostBodyEventTypesItem>|null
     */
    protected $eventTypes;
    /**
     * @var bool|null
     */
    protected $removeUnlisted;
    /**
     * @return string|null
     */
    public function getApplicationCode(): ?string
    {
        return $this->applicationCode;
    }
    /**
     * @param string|null $applicationCode
     *
     * @return self
     */
    public function setApplicationCode(?string $applicationCode): self
    {
        $this->initialized['applicationCode'] = true;
        $this->applicationCode = $applicationCode;
        return $this;
    }
    /**
     * @return list<ApiAdminEventTypesSyncPostBodyEventTypesItem>|null
     */
    public function getEventTypes(): ?array
    {
        return $this->eventTypes;
    }
    /**
     * @param list<ApiAdminEventTypesSyncPostBodyEventTypesItem>|null $eventTypes
     *
     * @return self
     */
    public function setEventTypes(?array $eventTypes): self
    {
        $this->initialized['eventTypes'] = true;
        $this->eventTypes = $eventTypes;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getRemoveUnlisted(): ?bool
    {
        return $this->removeUnlisted;
    }
    /**
     * @param bool|null $removeUnlisted
     *
     * @return self
     */
    public function setRemoveUnlisted(?bool $removeUnlisted): self
    {
        $this->initialized['removeUnlisted'] = true;
        $this->removeUnlisted = $removeUnlisted;
        return $this;
    }
}