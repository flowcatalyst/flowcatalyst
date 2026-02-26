<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminRolesPermissionsPermissionGetResponse200 extends \ArrayObject
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
     * @var string|null
     */
    protected $code;
    /**
     * @var string|null
     */
    protected $subdomain;
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
     * @var mixed|null
     */
    protected $description;
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
     * @return string|null
     */
    public function getSubdomain(): ?string
    {
        return $this->subdomain;
    }
    /**
     * @param string|null $subdomain
     *
     * @return self
     */
    public function setSubdomain(?string $subdomain): self
    {
        $this->initialized['subdomain'] = true;
        $this->subdomain = $subdomain;
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
    /**
     * @return mixed
     */
    public function getDescription()
    {
        return $this->description;
    }
    /**
     * @param mixed $description
     *
     * @return self
     */
    public function setDescription($description): self
    {
        $this->initialized['description'] = true;
        $this->description = $description;
        return $this;
    }
}