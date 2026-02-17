<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminPrincipalsIdApplicationAccessPutResponse200ApplicationsItem extends \ArrayObject
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
    protected $applicationId;
    /**
     * @var mixed|null
     */
    protected $applicationCode;
    /**
     * @var mixed|null
     */
    protected $applicationName;
    /**
     * @var mixed|null
     */
    protected $grantedAt;
    /**
     * @return string|null
     */
    public function getApplicationId(): ?string
    {
        return $this->applicationId;
    }
    /**
     * @param string|null $applicationId
     *
     * @return self
     */
    public function setApplicationId(?string $applicationId): self
    {
        $this->initialized['applicationId'] = true;
        $this->applicationId = $applicationId;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getApplicationCode()
    {
        return $this->applicationCode;
    }
    /**
     * @param mixed $applicationCode
     *
     * @return self
     */
    public function setApplicationCode($applicationCode): self
    {
        $this->initialized['applicationCode'] = true;
        $this->applicationCode = $applicationCode;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getApplicationName()
    {
        return $this->applicationName;
    }
    /**
     * @param mixed $applicationName
     *
     * @return self
     */
    public function setApplicationName($applicationName): self
    {
        $this->initialized['applicationName'] = true;
        $this->applicationName = $applicationName;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getGrantedAt()
    {
        return $this->grantedAt;
    }
    /**
     * @param mixed $grantedAt
     *
     * @return self
     */
    public function setGrantedAt($grantedAt): self
    {
        $this->initialized['grantedAt'] = true;
        $this->grantedAt = $grantedAt;
        return $this;
    }
}