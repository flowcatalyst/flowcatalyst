<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminClientsIdSuspendPostBody extends \ArrayObject
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
    protected $reason;
    /**
     * @var mixed|null
     */
    protected $note;
    /**
     * @return mixed
     */
    public function getReason()
    {
        return $this->reason;
    }
    /**
     * @param mixed $reason
     *
     * @return self
     */
    public function setReason($reason): self
    {
        $this->initialized['reason'] = true;
        $this->reason = $reason;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getNote()
    {
        return $this->note;
    }
    /**
     * @param mixed $note
     *
     * @return self
     */
    public function setNote($note): self
    {
        $this->initialized['note'] = true;
        $this->note = $note;
        return $this;
    }
}