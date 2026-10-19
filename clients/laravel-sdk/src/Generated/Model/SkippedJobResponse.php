<?php

namespace FlowCatalyst\Generated\Model;

class SkippedJobResponse
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
    protected $id;
    /**
     * IN_FLIGHT: QUEUED or PROCESSING, never reset. COMPLETED_REDRIVE_NOT_CONFIRMED: completed jobs are redriven only with redriveCompleted and confirmRedriveCompleted.
     *
     * @var string|null
     */
    protected $reason;
    /**
     * @var string|null
     */
    protected $status;
    /**
     * @return string|null
     */
    public function getId(): ?string
    {
        return $this->id;
    }
    /**
     * @param string|null $id
     *
     * @return self
     */
    public function setId(?string $id): self
    {
        $this->initialized['id'] = true;
        $this->id = $id;
        return $this;
    }
    /**
     * IN_FLIGHT: QUEUED or PROCESSING, never reset. COMPLETED_REDRIVE_NOT_CONFIRMED: completed jobs are redriven only with redriveCompleted and confirmRedriveCompleted.
     *
     * @return string|null
     */
    public function getReason(): ?string
    {
        return $this->reason;
    }
    /**
     * IN_FLIGHT: QUEUED or PROCESSING, never reset. COMPLETED_REDRIVE_NOT_CONFIRMED: completed jobs are redriven only with redriveCompleted and confirmRedriveCompleted.
     *
     * @param string|null $reason
     *
     * @return self
     */
    public function setReason(?string $reason): self
    {
        $this->initialized['reason'] = true;
        $this->reason = $reason;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getStatus(): ?string
    {
        return $this->status;
    }
    /**
     * @param string|null $status
     *
     * @return self
     */
    public function setStatus(?string $status): self
    {
        $this->initialized['status'] = true;
        $this->status = $status;
        return $this;
    }
}