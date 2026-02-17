<?php

namespace FlowCatalyst\Generated\Model;

class ApiSdkClientsIdActivatePostBody extends \ArrayObject
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
}