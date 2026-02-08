<?php

namespace FlowCatalyst\Generated\Model;

class PermissionInputDto extends \ArrayObject
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
     * Application code
     *
     * @var string|null
     */
    protected $application;
    /**
     * Bounded context
     *
     * @var string|null
     */
    protected $context;
    /**
     * Resource/entity
     *
     * @var string|null
     */
    protected $aggregate;
    /**
     * Operation
     *
     * @var string|null
     */
    protected $action;
    /**
     * Application code
     *
     * @return string|null
     */
    public function getApplication(): ?string
    {
        return $this->application;
    }
    /**
     * Application code
     *
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
     * Bounded context
     *
     * @return string|null
     */
    public function getContext(): ?string
    {
        return $this->context;
    }
    /**
     * Bounded context
     *
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
     * Resource/entity
     *
     * @return string|null
     */
    public function getAggregate(): ?string
    {
        return $this->aggregate;
    }
    /**
     * Resource/entity
     *
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
     * Operation
     *
     * @return string|null
     */
    public function getAction(): ?string
    {
        return $this->action;
    }
    /**
     * Operation
     *
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