<?php

namespace FlowCatalyst\Generated\Model;

class PermissionInputDto3 extends \ArrayObject
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
    protected $application;
    /**
     * @var string|null
     */
    protected $context;
    /**
     * @var string|null
     */
    protected $aggregate;
    /**
     * @var string|null
     */
    protected $action;
    /**
     * @return string|null
     */
    public function getApplication(): ?string
    {
        return $this->application;
    }
    /**
     * @param string|null $application
     *
     * @return self
     */
    public function setApplication(?string $application): self
    {
        $this->initialized['application'] = true;
        $this->application = $application;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getContext(): ?string
    {
        return $this->context;
    }
    /**
     * @param string|null $context
     *
     * @return self
     */
    public function setContext(?string $context): self
    {
        $this->initialized['context'] = true;
        $this->context = $context;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getAggregate(): ?string
    {
        return $this->aggregate;
    }
    /**
     * @param string|null $aggregate
     *
     * @return self
     */
    public function setAggregate(?string $aggregate): self
    {
        $this->initialized['aggregate'] = true;
        $this->aggregate = $aggregate;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getAction(): ?string
    {
        return $this->action;
    }
    /**
     * @param string|null $action
     *
     * @return self
     */
    public function setAction(?string $action): self
    {
        $this->initialized['action'] = true;
        $this->action = $action;
        return $this;
    }
}