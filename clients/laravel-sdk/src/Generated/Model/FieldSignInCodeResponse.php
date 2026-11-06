<?php

namespace FlowCatalyst\Generated\Model;

class FieldSignInCodeResponse
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
     * @var string|null
     */
    protected $appCode;
    /**
     * @var string|null
     */
    protected $code;
    /**
     * @var \DateTime|null
     */
    protected $expiresAt;
    /**
     * @var string|null
     */
    protected $workerId;
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
     * @return string|null
     */
    public function getCode(): ?string
    {
        return $this->code;
    }
    /**
     * @param string|null $code
     *
     * @return self
     */
    public function setCode(?string $code): self
    {
        $this->initialized['code'] = true;
        $this->code = $code;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getExpiresAt(): ?\DateTime
    {
        return $this->expiresAt;
    }
    /**
     * @param \DateTime|null $expiresAt
     *
     * @return self
     */
    public function setExpiresAt(?\DateTime $expiresAt): self
    {
        $this->initialized['expiresAt'] = true;
        $this->expiresAt = $expiresAt;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getWorkerId(): ?string
    {
        return $this->workerId;
    }
    /**
     * @param string|null $workerId
     *
     * @return self
     */
    public function setWorkerId(?string $workerId): self
    {
        $this->initialized['workerId'] = true;
        $this->workerId = $workerId;
        return $this;
    }
}