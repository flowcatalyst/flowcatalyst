<?php

namespace FlowCatalyst\Generated\Model;

class Version
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
     * @var int|null
     */
    protected $abi;
    /**
     * @var mixed|null
     */
    protected $describe;
    /**
     * @var string|null
     */
    protected $digest;
    /**
     * @var int|null
     */
    protected $number;
    /**
     * @var list<string>|null
     */
    protected $roles;
    /**
     * @var string|null
     */
    protected $runtime;
    /**
     * @return int|null
     */
    public function getAbi(): ?int
    {
        return $this->abi;
    }
    /**
     * @param int|null $abi
     *
     * @return self
     */
    public function setAbi(?int $abi): self
    {
        $this->initialized['abi'] = true;
        $this->abi = $abi;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getDescribe()
    {
        return $this->describe;
    }
    /**
     * @param mixed $describe
     *
     * @return self
     */
    public function setDescribe($describe): self
    {
        $this->initialized['describe'] = true;
        $this->describe = $describe;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getDigest(): ?string
    {
        return $this->digest;
    }
    /**
     * @param string|null $digest
     *
     * @return self
     */
    public function setDigest(?string $digest): self
    {
        $this->initialized['digest'] = true;
        $this->digest = $digest;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getNumber(): ?int
    {
        return $this->number;
    }
    /**
     * @param int|null $number
     *
     * @return self
     */
    public function setNumber(?int $number): self
    {
        $this->initialized['number'] = true;
        $this->number = $number;
        return $this;
    }
    /**
     * @return list<string>|null
     */
    public function getRoles(): ?array
    {
        return $this->roles;
    }
    /**
     * @param list<string>|null $roles
     *
     * @return self
     */
    public function setRoles(?array $roles): self
    {
        $this->initialized['roles'] = true;
        $this->roles = $roles;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getRuntime(): ?string
    {
        return $this->runtime;
    }
    /**
     * @param string|null $runtime
     *
     * @return self
     */
    public function setRuntime(?string $runtime): self
    {
        $this->initialized['runtime'] = true;
        $this->runtime = $runtime;
        return $this;
    }
}