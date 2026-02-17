<?php

namespace FlowCatalyst\Generated\Model;

class ApiBffRolesFiltersApplicationsGetResponse200 extends \ArrayObject
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
     * @var list<ApiBffRolesFiltersApplicationsGetResponse200OptionsItem>|null
     */
    protected $options;
    /**
     * @return list<ApiBffRolesFiltersApplicationsGetResponse200OptionsItem>|null
     */
    public function getOptions(): ?array
    {
        return $this->options;
    }
    /**
     * @param list<ApiBffRolesFiltersApplicationsGetResponse200OptionsItem>|null $options
     *
     * @return self
     */
    public function setOptions(?array $options): self
    {
        $this->initialized['options'] = true;
        $this->options = $options;
        return $this;
    }
}