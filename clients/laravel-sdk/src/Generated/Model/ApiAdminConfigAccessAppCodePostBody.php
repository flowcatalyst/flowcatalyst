<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminConfigAccessAppCodePostBody extends \ArrayObject
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
    protected $roleCode;
    /**
     * @var bool|null
     */
    protected $canRead;
    /**
     * @var bool|null
     */
    protected $canWrite;
    /**
     * @return string|null
     */
    public function getRoleCode(): ?string
    {
        return $this->roleCode;
    }
    /**
     * @param string|null $roleCode
     *
     * @return self
     */
    public function setRoleCode(?string $roleCode): self
    {
        $this->initialized['roleCode'] = true;
        $this->roleCode = $roleCode;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getCanRead(): ?bool
    {
        return $this->canRead;
    }
    /**
     * @param bool|null $canRead
     *
     * @return self
     */
    public function setCanRead(?bool $canRead): self
    {
        $this->initialized['canRead'] = true;
        $this->canRead = $canRead;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getCanWrite(): ?bool
    {
        return $this->canWrite;
    }
    /**
     * @param bool|null $canWrite
     *
     * @return self
     */
    public function setCanWrite(?bool $canWrite): self
    {
        $this->initialized['canWrite'] = true;
        $this->canWrite = $canWrite;
        return $this;
    }
}