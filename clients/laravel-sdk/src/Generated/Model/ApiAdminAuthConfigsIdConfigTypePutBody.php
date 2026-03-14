<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminAuthConfigsIdConfigTypePutBody extends \ArrayObject
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
     * @var mixed|null
     */
    protected $configType;
    /**
     * @var mixed|null
     */
    protected $primaryClientId;
    /**
     * @return mixed
     */
    public function getConfigType()
    {
        return $this->configType;
    }
    /**
     * @param mixed $configType
     *
     * @return self
     */
    public function setConfigType($configType): self
    {
        $this->initialized['configType'] = true;
        $this->configType = $configType;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getPrimaryClientId()
    {
        return $this->primaryClientId;
    }
    /**
     * @param mixed $primaryClientId
     *
     * @return self
     */
    public function setPrimaryClientId($primaryClientId): self
    {
        $this->initialized['primaryClientId'] = true;
        $this->primaryClientId = $primaryClientId;
        return $this;
    }
}