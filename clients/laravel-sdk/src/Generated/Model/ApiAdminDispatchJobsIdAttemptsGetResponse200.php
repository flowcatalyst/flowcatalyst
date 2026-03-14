<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminDispatchJobsIdAttemptsGetResponse200 extends \ArrayObject
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
    protected $dispatchJobId;
    /**
     * @var list<ApiAdminDispatchJobsIdAttemptsGetResponse200AttemptsItem>|null
     */
    protected $attempts;
    /**
     * @var int|null
     */
    protected $total;
    /**
     * @return string|null
     */
    public function getDispatchJobId(): ?string
    {
        return $this->dispatchJobId;
    }
    /**
     * @param string|null $dispatchJobId
     *
     * @return self
     */
    public function setDispatchJobId(?string $dispatchJobId): self
    {
        $this->initialized['dispatchJobId'] = true;
        $this->dispatchJobId = $dispatchJobId;
        return $this;
    }
    /**
     * @return list<ApiAdminDispatchJobsIdAttemptsGetResponse200AttemptsItem>|null
     */
    public function getAttempts(): ?array
    {
        return $this->attempts;
    }
    /**
     * @param list<ApiAdminDispatchJobsIdAttemptsGetResponse200AttemptsItem>|null $attempts
     *
     * @return self
     */
    public function setAttempts(?array $attempts): self
    {
        $this->initialized['attempts'] = true;
        $this->attempts = $attempts;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getTotal(): ?int
    {
        return $this->total;
    }
    /**
     * @param int|null $total
     *
     * @return self
     */
    public function setTotal(?int $total): self
    {
        $this->initialized['total'] = true;
        $this->total = $total;
        return $this;
    }
}