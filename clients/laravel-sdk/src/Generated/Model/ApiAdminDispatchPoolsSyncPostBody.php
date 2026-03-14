<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminDispatchPoolsSyncPostBody extends \ArrayObject
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
     * @var list<ApiAdminDispatchPoolsSyncPostBodyPoolsItem>|null
     */
    protected $pools;
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
     * @return list<ApiAdminDispatchPoolsSyncPostBodyPoolsItem>|null
     */
    public function getPools(): ?array
    {
        return $this->pools;
    }
    /**
     * @param list<ApiAdminDispatchPoolsSyncPostBodyPoolsItem>|null $pools
     *
     * @return self
     */
    public function setPools(?array $pools): self
    {
        $this->initialized['pools'] = true;
        $this->pools = $pools;
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