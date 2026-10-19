<?php

namespace FlowCatalyst\Generated\Model;

class RequeueRequest extends \ArrayObject
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
     * The literal REDRIVE, confirming redriveCompleted. Without both fields, COMPLETED jobs are skipped.
     *
     * @var string|null
     */
    protected $confirmRedriveCompleted;
    /**
     * Dispatch job ids to reset to PENDING for re-dispatch. QUEUED and PROCESSING jobs are never reset; COMPLETED jobs only with redriveCompleted and confirmRedriveCompleted. Skipped jobs are reported, not an error.
     *
     * @var list<string>|null
     */
    protected $ids;
    /**
     * Also re-deliver COMPLETED jobs. Needs confirmRedriveCompleted=REDRIVE and the dispatch-job:redrive-completed permission (403 without it).
     *
     * @var bool|null
     */
    protected $redriveCompleted;
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
     * The literal REDRIVE, confirming redriveCompleted. Without both fields, COMPLETED jobs are skipped.
     *
     * @return string|null
     */
    public function getConfirmRedriveCompleted(): ?string
    {
        return $this->confirmRedriveCompleted;
    }
    /**
     * The literal REDRIVE, confirming redriveCompleted. Without both fields, COMPLETED jobs are skipped.
     *
     * @param string|null $confirmRedriveCompleted
     *
     * @return self
     */
    public function setConfirmRedriveCompleted(?string $confirmRedriveCompleted): self
    {
        $this->initialized['confirmRedriveCompleted'] = true;
        $this->confirmRedriveCompleted = $confirmRedriveCompleted;
        return $this;
    }
    /**
     * Dispatch job ids to reset to PENDING for re-dispatch. QUEUED and PROCESSING jobs are never reset; COMPLETED jobs only with redriveCompleted and confirmRedriveCompleted. Skipped jobs are reported, not an error.
     *
     * @return list<string>|null
     */
    public function getIds(): ?array
    {
        return $this->ids;
    }
    /**
     * Dispatch job ids to reset to PENDING for re-dispatch. QUEUED and PROCESSING jobs are never reset; COMPLETED jobs only with redriveCompleted and confirmRedriveCompleted. Skipped jobs are reported, not an error.
     *
     * @param list<string>|null $ids
     *
     * @return self
     */
    public function setIds(?array $ids): self
    {
        $this->initialized['ids'] = true;
        $this->ids = $ids;
        return $this;
    }
    /**
     * Also re-deliver COMPLETED jobs. Needs confirmRedriveCompleted=REDRIVE and the dispatch-job:redrive-completed permission (403 without it).
     *
     * @return bool|null
     */
    public function getRedriveCompleted(): ?bool
    {
        return $this->redriveCompleted;
    }
    /**
     * Also re-deliver COMPLETED jobs. Needs confirmRedriveCompleted=REDRIVE and the dispatch-job:redrive-completed permission (403 without it).
     *
     * @param bool|null $redriveCompleted
     *
     * @return self
     */
    public function setRedriveCompleted(?bool $redriveCompleted): self
    {
        $this->initialized['redriveCompleted'] = true;
        $this->redriveCompleted = $redriveCompleted;
        return $this;
    }
}