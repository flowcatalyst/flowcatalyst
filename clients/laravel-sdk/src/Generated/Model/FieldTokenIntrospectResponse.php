<?php

namespace FlowCatalyst\Generated\Model;

class FieldTokenIntrospectResponse
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
     * @var FieldTokenActor|null
     */
    protected $act;
    /**
     * @var bool|null
     */
    protected $active;
    /**
     * @var list<string>|null
     */
    protected $amr;
    /**
     * @var string|null
     */
    protected $appCode;
    /**
     * @var \DateTime|null
     */
    protected $authTime;
    /**
     * @var string|null
     */
    protected $clientId;
    /**
     * @var string|null
     */
    protected $deviceId;
    /**
     * @var \DateTime|null
     */
    protected $exp;
    /**
     * @var string|null
     */
    protected $linkedPrincipalId;
    /**
     * @var string|null
     */
    protected $sub;
    /**
     * @var bool|null
     */
    protected $test;
    /**
     * @var string|null
     */
    protected $workerCode;
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
     * @return FieldTokenActor|null
     */
    public function getAct(): ?FieldTokenActor
    {
        return $this->act;
    }
    /**
     * @param FieldTokenActor|null $act
     *
     * @return self
     */
    public function setAct(?FieldTokenActor $act): self
    {
        $this->initialized['act'] = true;
        $this->act = $act;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getActive(): ?bool
    {
        return $this->active;
    }
    /**
     * @param bool|null $active
     *
     * @return self
     */
    public function setActive(?bool $active): self
    {
        $this->initialized['active'] = true;
        $this->active = $active;
        return $this;
    }
    /**
     * @return list<string>|null
     */
    public function getAmr(): ?array
    {
        return $this->amr;
    }
    /**
     * @param list<string>|null $amr
     *
     * @return self
     */
    public function setAmr(?array $amr): self
    {
        $this->initialized['amr'] = true;
        $this->amr = $amr;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getAppCode(): ?string
    {
        return $this->appCode;
    }
    /**
     * @param string|null $appCode
     *
     * @return self
     */
    public function setAppCode(?string $appCode): self
    {
        $this->initialized['appCode'] = true;
        $this->appCode = $appCode;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getAuthTime(): ?\DateTime
    {
        return $this->authTime;
    }
    /**
     * @param \DateTime|null $authTime
     *
     * @return self
     */
    public function setAuthTime(?\DateTime $authTime): self
    {
        $this->initialized['authTime'] = true;
        $this->authTime = $authTime;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getClientId(): ?string
    {
        return $this->clientId;
    }
    /**
     * @param string|null $clientId
     *
     * @return self
     */
    public function setClientId(?string $clientId): self
    {
        $this->initialized['clientId'] = true;
        $this->clientId = $clientId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getDeviceId(): ?string
    {
        return $this->deviceId;
    }
    /**
     * @param string|null $deviceId
     *
     * @return self
     */
    public function setDeviceId(?string $deviceId): self
    {
        $this->initialized['deviceId'] = true;
        $this->deviceId = $deviceId;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getExp(): ?\DateTime
    {
        return $this->exp;
    }
    /**
     * @param \DateTime|null $exp
     *
     * @return self
     */
    public function setExp(?\DateTime $exp): self
    {
        $this->initialized['exp'] = true;
        $this->exp = $exp;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getLinkedPrincipalId(): ?string
    {
        return $this->linkedPrincipalId;
    }
    /**
     * @param string|null $linkedPrincipalId
     *
     * @return self
     */
    public function setLinkedPrincipalId(?string $linkedPrincipalId): self
    {
        $this->initialized['linkedPrincipalId'] = true;
        $this->linkedPrincipalId = $linkedPrincipalId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getSub(): ?string
    {
        return $this->sub;
    }
    /**
     * @param string|null $sub
     *
     * @return self
     */
    public function setSub(?string $sub): self
    {
        $this->initialized['sub'] = true;
        $this->sub = $sub;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getTest(): ?bool
    {
        return $this->test;
    }
    /**
     * @param bool|null $test
     *
     * @return self
     */
    public function setTest(?bool $test): self
    {
        $this->initialized['test'] = true;
        $this->test = $test;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getWorkerCode(): ?string
    {
        return $this->workerCode;
    }
    /**
     * @param string|null $workerCode
     *
     * @return self
     */
    public function setWorkerCode(?string $workerCode): self
    {
        $this->initialized['workerCode'] = true;
        $this->workerCode = $workerCode;
        return $this;
    }
}