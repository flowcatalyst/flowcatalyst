<?php

namespace FlowCatalyst\Generated\Model;

class BulkIssuedFieldBadge
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
     * @var FieldBadgeResponse|null
     */
    protected $badge;
    /**
     * @var string|null
     */
    protected $name;
    /**
     * @var string|null
     */
    protected $token;
    /**
     * @var string|null
     */
    protected $workerCode;
    /**
     * @var string|null
     */
    protected $workerId;
    /**
     * @return FieldBadgeResponse|null
     */
    public function getBadge(): ?FieldBadgeResponse
    {
        return $this->badge;
    }
    /**
     * @param FieldBadgeResponse|null $badge
     *
     * @return self
     */
    public function setBadge(?FieldBadgeResponse $badge): self
    {
        $this->initialized['badge'] = true;
        $this->badge = $badge;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getName(): ?string
    {
        return $this->name;
    }
    /**
     * @param string|null $name
     *
     * @return self
     */
    public function setName(?string $name): self
    {
        $this->initialized['name'] = true;
        $this->name = $name;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getToken(): ?string
    {
        return $this->token;
    }
    /**
     * @param string|null $token
     *
     * @return self
     */
    public function setToken(?string $token): self
    {
        $this->initialized['token'] = true;
        $this->token = $token;
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